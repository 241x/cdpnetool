package handler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cdpnetool/internal/executor"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/domain"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/fetch"
)

// Handler 事件处理器，负责协调规则匹配、行为执行和全周期事件合并
type Handler struct {
	engine            *rules.Engine
	events            chan domain.NetworkEvent
	processTimeoutMS  int
	bodySizeThreshold int64 // 允许采集和修改响应体的最大限制
	log               logger.Logger
	collectUnmatched  bool     // 是否收集未匹配的请求
	pendingPool       sync.Map // 在途请求池: map[RequestID]*PendingRequest
}

// PendingRequest 暂存在内存中的请求阶段信息
type PendingRequest struct {
	TraceID         string
	StartTime       time.Time
	RequestInfo     domain.RequestInfo
	MatchedRules    []domain.RuleMatch
	RawMatchedRules []*rules.MatchedRule
	IsMatched       bool
	RequestModified bool
	Committed       uint32
}

// Config 配置选项
type Config struct {
	Engine            *rules.Engine
	Events            chan domain.NetworkEvent
	ProcessTimeoutMS  int
	BodySizeThreshold int64
	Logger            logger.Logger
	CollectUnmatched  bool
}

// New 创建事件处理器并启动清理协程
func New(cfg Config) *Handler {
	h := &Handler{
		engine:            cfg.Engine,
		events:            cfg.Events,
		processTimeoutMS:  cfg.ProcessTimeoutMS,
		bodySizeThreshold: cfg.BodySizeThreshold,
		log:               cfg.Logger,
		collectUnmatched:  cfg.CollectUnmatched,
	}

	go h.cleanupLoop()
	return h
}

// SetCollectUnmatched 动态设置是否收集未匹配请求
func (h *Handler) SetCollectUnmatched(collect bool) {
	h.collectUnmatched = collect
}

// SetEngine 设置规则引擎
func (h *Handler) SetEngine(engine *rules.Engine) {
	h.engine = engine
}

// SetProcessTimeout 设置处理超时时间
func (h *Handler) SetProcessTimeout(timeoutMS int) {
	h.processTimeoutMS = timeoutMS
}

// cleanupLoop 定期清理内存池中的孤儿请求（防止由于浏览器异常导致的数据残留）
func (h *Handler) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		h.pendingPool.Range(func(key, value any) bool {
			req, ok := value.(*PendingRequest)
			if ok && now.Sub(req.StartTime) > 60*time.Second {
				h.pendingPool.Delete(key)
				h.log.Debug("清理过期请求记录", "requestID", key, "traceID", req.TraceID)
			}
			return true
		})
	}
}

// HandleRequest 处理请求拦截
func (h *Handler) HandleRequest(
	ctx context.Context,
	targetID domain.TargetID,
	client *cdp.Client,
	ev *fetch.RequestPausedReply,
	l logger.Logger,
	traceID string,
) {
	evalCtx := executor.ToEvalContext(ev)
	if h.engine == nil {
		if h.collectUnmatched {
			h.saveUnmatchedToPool(ev, traceID)
		}
		h.safeContinueRequest(ctx, client, ev.RequestID)
		return
	}

	start := time.Now()

	// 创建执行器
	exec := executor.New(l, ev, executor.Options{
		MaxCaptureSize: h.bodySizeThreshold,
		ProcessTimeout: time.Duration(h.processTimeoutMS) * time.Millisecond,
	})

	// 评估所有匹配规则
	allMatchedRules := h.engine.Eval(evalCtx)
	h.engine.RecordStats(allMatchedRules)
	isMatched := len(allMatchedRules) > 0
	ruleMatches := buildRuleMatches(allMatchedRules)

	if !isMatched {
		if h.collectUnmatched {
			h.saveUnmatchedToPool(ev, traceID)
		}
		h.safeContinueRequest(ctx, client, ev.RequestID)
		return
	}

	// 执行请求阶段行为
	res := exec.ExecuteRequest(allMatchedRules)

	// 场景：阻止 (Block)
	if res.IsBlocked {
		if res.FulfillArgs != nil {
			if err := client.Fetch.FulfillRequest(ctx, res.FulfillArgs); err != nil {
				l.Err(err, "执行 FulfillRequest (Block) 失败，尝试保底继续", "requestID", ev.RequestID)
				h.safeContinueRequest(ctx, client, ev.RequestID)
			}
		}
		// 生成审计快照并发送
		reqInfo := exec.CaptureRequestSnapshot()
		h.sendMatchedEvent(targetID, "blocked", ruleMatches, reqInfo, domain.ResponseInfo{})
		l.Debug("请求被拦截阻止", "duration", time.Since(start))
		return
	}

	// 场景：继续 (Modified or Passed)
	if res.ContinueArgs != nil {
		if err := client.Fetch.ContinueRequest(ctx, res.ContinueArgs); err != nil {
			l.Err(err, "执行 ContinueRequest (Modified) 失败，尝试保底继续", "requestID", ev.RequestID)
			h.safeContinueRequest(ctx, client, ev.RequestID)
		}
	} else {
		h.safeContinueRequest(ctx, client, ev.RequestID)
	}

	// 长连接预判
	if res.IsLongConn {
		l.Debug("检测到长连接请求，跳过响应阶段，立即发送原子事件")
		reqInfo := exec.CaptureRequestSnapshot()
		result := "matched"
		if res.IsModified {
			result = "modified"
		}
		h.sendMatchedEvent(targetID, result, ruleMatches, reqInfo, domain.ResponseInfo{})
		return
	}

	// 入池暂存：保存上下文，等待响应阶段
	h.pendingPool.Store(ev.RequestID, &PendingRequest{
		TraceID:         traceID,
		StartTime:       start,
		RequestInfo:     exec.CaptureRequestSnapshot(),
		MatchedRules:    ruleMatches,
		RawMatchedRules: allMatchedRules,
		IsMatched:       true,
		RequestModified: res.IsModified,
	})
}

// HandleResponse 处理响应拦截
func (h *Handler) HandleResponse(
	client *cdp.Client,
	ctx context.Context,
	targetID domain.TargetID,
	ev *fetch.RequestPausedReply,
	l logger.Logger,
	traceID string,
) {
	val, ok := h.pendingPool.Load(ev.RequestID)
	if !ok {
		h.safeContinueResponse(ctx, client, ev.RequestID)
		return
	}
	pending := val.(*PendingRequest)
	defer h.pendingPool.Delete(ev.RequestID)

	// 避免重复处理
	if !atomic.CompareAndSwapUint32(&pending.Committed, 0, 1) {
		return
	}

	l = l.With("traceID", pending.TraceID)
	exec := executor.New(l, ev, executor.Options{
		MaxCaptureSize: h.bodySizeThreshold,
	})

	// 获取响应体与熔断检查
	var originalBody string
	isUnsafe, reason := exec.IsUnsafeResponseBody()
	if isUnsafe {
		l.Info("响应负载熔断", "reason", reason)
		originalBody = fmt.Sprintf("[Body omitted: %s]", reason)
	} else {
		var err error
		originalBody, err = exec.FetchResponseBody(ctx, client)
		if err != nil {
			l.Warn("获取响应体失败，审计数据将不完整", "err", err)
			originalBody = "[Error: failed to fetch response body]"
		}
	}

	// 执行响应阶段行为
	var finalResult string = "passed"
	var res *executor.ExecutionResult

	if pending.IsMatched {
		res = exec.ExecuteResponse(pending.RawMatchedRules, originalBody)

		if res.FulfillArgs != nil {
			if err := client.Fetch.FulfillRequest(ctx, res.FulfillArgs); err != nil {
				l.Err(err, "执行 FulfillRequest (Modified Response) 失败，尝试保底继续")
				h.safeContinueResponse(ctx, client, ev.RequestID)
			}
			finalResult = "modified"
		} else if res.ContinueRes != nil {
			if err := client.Fetch.ContinueResponse(ctx, res.ContinueRes); err != nil {
				l.Err(err, "执行 ContinueResponse (Modified Response) 失败，尝试保底继续")
				h.safeContinueResponse(ctx, client, ev.RequestID)
			}
			finalResult = "modified"
		} else {
			h.safeContinueResponse(ctx, client, ev.RequestID)
			if pending.RequestModified {
				finalResult = "modified"
			} else {
				finalResult = "matched"
			}
		}
	} else {
		h.safeContinueResponse(ctx, client, ev.RequestID)
	}

	// 发送全周期审计事件
	finalBody := originalBody
	if res != nil && res.IsModified {
		// 如果有修改，尝试获取修改后的 body（ExecuteResponse 内部已处理）
		finalBody = exec.CaptureResponseSnapshot(originalBody).Body
	}

	reqInfo := pending.RequestInfo
	resInfo := exec.CaptureResponseSnapshot(finalBody)
	h.sendMatchedEvent(targetID, finalResult, pending.MatchedRules, reqInfo, resInfo)
}

// sendMatchedEvent 统一发送网络事件
func (h *Handler) sendMatchedEvent(
	targetID domain.TargetID,
	finalResult string,
	matchedRules []domain.RuleMatch,
	requestInfo domain.RequestInfo,
	responseInfo domain.ResponseInfo,
) {
	if h.events == nil {
		return
	}

	evt := domain.NetworkEvent{
		Session:      "", // 会在上层填充
		Target:       targetID,
		Timestamp:    time.Now().UnixMilli(),
		IsMatched:    len(matchedRules) > 0,
		Request:      requestInfo,
		Response:     responseInfo,
		FinalResult:  finalResult,
		MatchedRules: matchedRules,
	}

	select {
	case h.events <- evt:
	default:
	}
}

// saveUnmatchedToPool 快速保存未匹配的请求信息到池
func (h *Handler) saveUnmatchedToPool(ev *fetch.RequestPausedReply, traceID string) {
	// 临时创建一个执行器来捕获快照，不带动作执行
	exec := executor.New(h.log, ev, executor.Options{})
	h.pendingPool.Store(ev.RequestID, &PendingRequest{
		TraceID:     traceID,
		StartTime:   time.Now(),
		RequestInfo: exec.CaptureRequestSnapshot(),
		IsMatched:   false,
	})
}

// buildRuleMatches 构建规则匹配信息列表
func buildRuleMatches(matchedRules []*rules.MatchedRule) []domain.RuleMatch {
	matches := make([]domain.RuleMatch, len(matchedRules))
	for i, mr := range matchedRules {
		actionTypes := make([]string, 0, len(mr.Rule.Actions))
		for _, action := range mr.Rule.Actions {
			actionTypes = append(actionTypes, string(action.Type))
		}
		matches[i] = domain.RuleMatch{
			RuleID:   mr.Rule.ID,
			RuleName: mr.Rule.Name,
			Actions:  actionTypes,
		}
	}
	return matches
}

// safeContinueRequest 尝试保底继续请求
func (h *Handler) safeContinueRequest(ctx context.Context, client *cdp.Client, requestID fetch.RequestID) {
	err := client.Fetch.ContinueRequest(ctx, &fetch.ContinueRequestArgs{RequestID: requestID})
	if err != nil {
		h.log.Err(err, "保底 ContinueRequest 失败", "requestID", requestID)
	}
}

// safeContinueResponse 尝试保底继续响应
func (h *Handler) safeContinueResponse(ctx context.Context, client *cdp.Client, requestID fetch.RequestID) {
	err := client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: requestID})
	if err != nil {
		h.log.Err(err, "保底 ContinueResponse 失败", "requestID", requestID)
	}
}
