package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"cdpnetool/internal/adapter/cdp"
	"cdpnetool/internal/auditor"
	"cdpnetool/internal/engine"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/pool"
	"cdpnetool/internal/processor"
	"cdpnetool/internal/session"
	"cdpnetool/internal/tracker"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"

	"github.com/mafredri/cdp/protocol/fetch"
)

// sessionState 维护单个会话的所有新架构组件
type sessionState struct {
	id                  domain.SessionID
	cfg                 domain.SessionConfig
	sess                *session.Session
	clientMgr           *cdp.ClientManager
	interceptor         *cdp.Interceptor
	engine              *engine.Engine
	tracker             *tracker.Tracker
	matchedAuditor      *auditor.Auditor
	trafficAuditor      *auditor.Auditor
	processor           *processor.Processor
	events              chan domain.NetworkEvent
	trafficEvs          chan domain.NetworkEvent
	workPool            *pool.Pool
	ctx                 context.Context
	cancel              context.CancelFunc
	interceptionEnabled bool
	mu                  sync.Mutex
}

// Orchestrator 新架构业务编排器
type Orchestrator struct {
	mu       sync.RWMutex
	sessions map[domain.SessionID]*sessionState
	log      logger.Logger
}

// New 创建编排器实例
func New(l logger.Logger) *Orchestrator {
	if l == nil {
		l = logger.NewNop()
	}
	return &Orchestrator{
		sessions: make(map[domain.SessionID]*sessionState),
		log:      l,
	}
}

// StartSession 创建并启动一个新的拦截会话
func (o *Orchestrator) StartSession(ctx context.Context, cfg domain.SessionConfig) (domain.SessionID, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	id := domain.SessionID(fmt.Sprintf("osess_%d", time.Now().UnixNano()))

	sessionCtx, cancel := context.WithCancel(ctx)

	// 初始化会话级基础设施
	workPool := pool.New(cfg.Concurrency, cfg.PendingCapacity)
	workPool.Start(sessionCtx)

	events := make(chan domain.NetworkEvent, cfg.PendingCapacity)
	trafficChan := make(chan domain.NetworkEvent, cfg.PendingCapacity)

	// 初始化各层组件
	eng := engine.New(&rulespec.Config{})
	matchedAud := auditor.New(events, o.log)
	trafficAud := auditor.New(trafficChan, o.log)
	trk := tracker.New(time.Duration(cfg.ProcessTimeoutMS)*time.Millisecond, o.log)
	proc := processor.New(trk, eng, matchedAud, trafficAud, o.log)

	clientMgr := cdp.NewClientManager(cfg.DevToolsURL, o.log)

	// 验证连通性
	if err := clientMgr.TestConnection(sessionCtx); err != nil {
		cancel()
		workPool.Stop()
		o.log.Err(err, "连接浏览器失败", "url", cfg.DevToolsURL)
		return "", fmt.Errorf("无法连接到浏览器: %w", err)
	}

	intr := cdp.NewInterceptor(o.log, workPool)

	sess := session.New(id)

	state := &sessionState{
		id:             id,
		cfg:            cfg,
		sess:           sess,
		clientMgr:      clientMgr,
		interceptor:    intr,
		engine:         eng,
		tracker:        trk,
		matchedAuditor: matchedAud,
		trafficAuditor: trafficAud,
		processor:      proc,
		events:         events,
		trafficEvs:     trafficChan,
		workPool:       workPool,
		ctx:            sessionCtx,
		cancel:         cancel,
	}

	o.sessions[id] = state
	o.log.Info("新架构会话已启动", "sessionID", string(id), "devtools", cfg.DevToolsURL)
	return id, nil
}

// StopSession 停止并清理指定的会话
func (o *Orchestrator) StopSession(ctx context.Context, id domain.SessionID) error {
	o.mu.Lock()
	state, ok := o.sessions[id]
	if ok {
		delete(o.sessions, id)
	}
	o.mu.Unlock()

	if !ok {
		return domain.ErrSessionNotFound
	}

	state.cancel()
	state.tracker.Stop()
	state.workPool.Stop()

	// 安全关闭 channel
	state.mu.Lock()
	select {
	case <-state.events:
	default:
		close(state.events)
	}
	select {
	case <-state.trafficEvs:
	default:
		close(state.trafficEvs)
	}
	state.mu.Unlock()

	o.log.Info("会话已停止", "sessionID", string(id))
	return nil
}

// AttachTarget 将指定目标附着到会话并启动事件监听
func (o *Orchestrator) AttachTarget(ctx context.Context, id domain.SessionID, target domain.TargetID) error {
	state, ok := o.get(id)
	if !ok {
		return domain.ErrSessionNotFound
	}

	ts, err := state.clientMgr.AttachTarget(ctx, target)
	if err != nil {
		return err
	}

	state.sess.AddTarget(target)

	// 启动 CDP 事件监听循环
	go state.interceptor.Consume(state.ctx, ts.Client, func(ev *fetch.RequestPausedReply) {
		o.handleEvent(state, ts, ev)
	})

	// 如果当前会话已开启拦截，物理启用该 Target 的拦截
	state.mu.Lock()
	enabled := state.interceptionEnabled
	state.mu.Unlock()

	if enabled {
		if err := state.interceptor.Enable(state.ctx, ts.Client); err != nil {
			o.log.Err(err, "Attach 时启用拦截失败", "target", string(target))
		}
	}

	return nil
}

// DetachTarget 断开指定目标与会话的连接
func (o *Orchestrator) DetachTarget(ctx context.Context, id domain.SessionID, target domain.TargetID) error {
	state, ok := o.get(id)
	if !ok {
		return domain.ErrSessionNotFound
	}
	state.sess.RemoveTarget(target)
	return state.clientMgr.DetachTarget(target)
}

// ListTargets 列出指定会话中的所有浏览器目标
func (o *Orchestrator) ListTargets(ctx context.Context, id domain.SessionID) ([]domain.TargetInfo, error) {
	state, ok := o.get(id)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return state.clientMgr.ListTargets(ctx)
}

// EnableInterception 开启指定会话的拦截功能
func (o *Orchestrator) EnableInterception(ctx context.Context, id domain.SessionID) error {
	state, ok := o.get(id)
	if !ok {
		return domain.ErrSessionNotFound
	}

	// 业务校验：至少需要附着一个目标
	targets := state.sess.GetTargets()
	if len(targets) == 0 {
		return domain.ErrNoTargetAttached
	}

	state.mu.Lock()
	state.interceptionEnabled = true
	state.mu.Unlock()

	// 遍历所有已附着的 Target 物理开启拦截
	for _, tid := range targets {
		ts, ok := state.clientMgr.GetSession(tid)
		if ok {
			if err := state.interceptor.Enable(ctx, ts.Client); err != nil {
				o.log.Err(err, "物理开启拦截失败", "target", string(tid))
			}
		}
	}
	o.log.Info("会话逻辑拦截已开启", "sessionID", string(id))
	return nil
}

// DisableInterception 关闭指定会话的拦截功能
func (o *Orchestrator) DisableInterception(ctx context.Context, id domain.SessionID) error {
	state, ok := o.get(id)
	if !ok {
		return domain.ErrSessionNotFound
	}

	state.mu.Lock()
	state.interceptionEnabled = false
	state.mu.Unlock()

	// 遍历所有已附着的 Target 物理关闭拦截
	targets := state.sess.GetTargets()
	for _, tid := range targets {
		ts, ok := state.clientMgr.GetSession(tid)
		if ok {
			if err := state.interceptor.Disable(ctx, ts.Client); err != nil {
				o.log.Err(err, "物理关闭拦截失败", "target", string(tid))
			}
		}
	}

	// 关闭工作池
	if state.workPool != nil {
		state.workPool.Stop()
	}

	o.log.Info("会话逻辑拦截已关闭", "sessionID", string(id))
	return nil
}

// LoadRules 加载规则配置到指定会话
func (o *Orchestrator) LoadRules(ctx context.Context, id domain.SessionID, cfg *rulespec.Config) error {
	state, ok := o.get(id)
	if !ok {
		return domain.ErrSessionNotFound
	}
	state.engine.Update(cfg)
	state.sess.UpdateConfig(cfg)
	return nil
}

// GetRuleStats 获取指定会话的规则匹配统计信息
func (o *Orchestrator) GetRuleStats(ctx context.Context, id domain.SessionID) (domain.EngineStats, error) {
	state, ok := o.get(id)
	if !ok {
		return domain.EngineStats{}, domain.ErrSessionNotFound
	}
	total, matched, byRule := state.engine.GetStats()
	stats := domain.EngineStats{
		Total:   total,
		Matched: matched,
		ByRule:  make(map[domain.RuleID]int64),
	}
	for k, v := range byRule {
		stats.ByRule[domain.RuleID(k)] = v
	}
	return stats, nil
}

// SubscribeEvents 订阅指定会话的事件流
func (o *Orchestrator) SubscribeEvents(ctx context.Context, id domain.SessionID) (<-chan domain.NetworkEvent, error) {
	state, ok := o.get(id)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return state.events, nil
}

// SubscribeTraffic 订阅指定会话的全量流量流
func (o *Orchestrator) SubscribeTraffic(ctx context.Context, id domain.SessionID) (<-chan domain.NetworkEvent, error) {
	state, ok := o.get(id)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return state.trafficEvs, nil
}

// EnableTrafficCapture 启用或禁用指定会话的流量捕获
func (o *Orchestrator) EnableTrafficCapture(ctx context.Context, id domain.SessionID, enabled bool) error {
	state, ok := o.get(id)
	if !ok {
		return domain.ErrSessionNotFound
	}
	state.trafficAuditor.SetEnabled(enabled)

	// 如果开启了捕获，且当前有附着目标，确保物理拦截已启用
	if !enabled {
		o.log.Info("更新流量捕获状态", "sessionID", string(id), "enabled", enabled)
		return nil
	}

	targets := state.sess.GetTargets()
	if len(targets) == 0 {
		o.log.Info("更新流量捕获状态", "sessionID", string(id), "enabled", enabled)
		return nil
	}

	for _, tid := range targets {
		ts, ok := state.clientMgr.GetSession(tid)
		if !ok {
			continue
		}
		if err := state.interceptor.Enable(ctx, ts.Client); err != nil {
			o.log.Err(err, "启用全量捕获时激活目标失败", "target", string(tid))
		}
	}

	o.log.Info("更新流量捕获状态", "sessionID", string(id), "enabled", enabled)
	return nil
}

// handleEvent 处理 CDP 原始事件并桥接到 Processor
func (o *Orchestrator) handleEvent(state *sessionState, ts *cdp.TargetSession, ev *fetch.RequestPausedReply) {
	stage := "request"
	if ev.ResponseStatusCode != nil {
		stage = "response"
	}
	o.log.Debug("[Orchestrator] 处理 CDP 事件", "requestID", ev.RequestID, "stage", stage, "url", ev.Request.URL, "method", ev.Request.Method)

	// 设置上下文
	state.processor.SetContext(string(state.id), string(ts.ID))

	if ev.ResponseStatusCode == nil {
		// 请求阶段
		req := cdp.ToNeutralRequest(ev)
		res := state.processor.ProcessRequest(state.ctx, req)
		o.log.Debug("[Orchestrator] 请求处理结果", "requestID", ev.RequestID, "action", res.Action)
		o.applyResult(state, ts, ev, res)
	} else {
		// 响应阶段
		// 获取原始响应体
		var body []byte
		ctx2, cancel2 := context.WithTimeout(state.ctx, 3*time.Second)
		rb, err := ts.Client.Fetch.GetResponseBody(ctx2, &fetch.GetResponseBodyArgs{RequestID: ev.RequestID})
		cancel2()
		if err != nil {
			o.log.Warn("获取响应体失败，执行降级放行", "requestID", ev.RequestID, "error", err.Error())
			if err := state.interceptor.ContinueResponse(state.ctx, ts.Client, ev.RequestID); err != nil {
				o.log.Err(err, "降级放行响应失败", "requestID", ev.RequestID)
			}
			return
		}
		if rb != nil {
			// GetResponseBody 返回的是 base64 编码的字符串，需要解码为原始字节
			if rb.Base64Encoded {
				decoded, err := base64.StdEncoding.DecodeString(rb.Body)
				if err != nil {
					o.log.Err(err, "解码响应体失败", "requestID", ev.RequestID)
					body = []byte(rb.Body)
				} else {
					body = decoded
				}
			} else {
				body = []byte(rb.Body)
			}
		}

		resp := cdp.ToNeutralResponse(ev, body)
		res := state.processor.ProcessResponse(state.ctx, string(ev.RequestID), resp)
		o.log.Debug("[Orchestrator] 响应处理结果", "requestID", ev.RequestID, "action", res.Action)
		o.applyResult(state, ts, ev, res)
	}
}

// applyResult 将中立处理结果反馈给物理适配层
func (o *Orchestrator) applyResult(state *sessionState, ts *cdp.TargetSession, ev *fetch.RequestPausedReply, res processor.Result) {
	id := ev.RequestID
	isRequest := ev.ResponseStatusCode == nil

	o.log.Debug("[Orchestrator] 开始应用结果", "requestID", id, "action", res.Action, "isRequest", isRequest)

	switch res.Action {
	case processor.ActionBlock:
		o.log.Info("[Orchestrator] 执行 Block 动作", "requestID", id, "statusCode", res.MockRes.StatusCode)
		// 无论请求还是响应阶段，拦截都通过 FulfillRequest 模拟响应
		if res.MockRes == nil {
			o.log.Err(nil, "Block 动作但 MockRes 为 nil，执行降级放行", "requestID", id)
			if isRequest {
				_ = state.interceptor.ContinueRequest(state.ctx, ts.Client, id)
			} else {
				_ = state.interceptor.ContinueResponse(state.ctx, ts.Client, id)
			}
			return
		}
		err := ts.Client.Fetch.FulfillRequest(state.ctx, &fetch.FulfillRequestArgs{
			RequestID:       id,
			ResponseCode:    res.MockRes.StatusCode,
			ResponseHeaders: cdp.ToHeaderEntries(res.MockRes.Headers),
			Body:            res.MockRes.Body,
		})
		if err != nil {
			o.log.Err(err, "[Orchestrator] 执行 Block 响应失败", "requestID", id)
		} else {
			o.log.Debug("[Orchestrator] Block 执行成功", "requestID", id)
		}

	case processor.ActionModify:
		o.log.Debug("[Orchestrator] 执行 Modify 动作", "requestID", id, "isRequest", isRequest)
		if isRequest {
			// 请求阶段修改
			err := ts.Client.Fetch.ContinueRequest(state.ctx, &fetch.ContinueRequestArgs{
				RequestID: id,
				URL:       &res.ModifiedReq.URL,
				Method:    &res.ModifiedReq.Method,
				Headers:   cdp.ToHeaderEntries(res.ModifiedReq.Headers),
				PostData:  res.ModifiedReq.Body,
			})
			if err != nil {
				o.log.Err(err, "[Orchestrator] 执行请求修改失败", "requestID", id)
			} else {
				o.log.Debug("[Orchestrator] 请求修改成功", "requestID", id)
			}
		} else {
			// 响应阶段修改：统一使用 FulfillRequest 全量覆盖
			code := 200
			var headers domain.Header
			var body []byte

			if res.ModifiedRes != nil {
				code = res.ModifiedRes.StatusCode
				headers = res.ModifiedRes.Headers
				body = res.ModifiedRes.Body
			} else {
				if ev.ResponseStatusCode != nil {
					code = *ev.ResponseStatusCode
				}
				headers = make(domain.Header)
				for _, h := range ev.ResponseHeaders {
					headers.Set(h.Name, h.Value)
				}
			}

			err := ts.Client.Fetch.FulfillRequest(state.ctx, &fetch.FulfillRequestArgs{
				RequestID:       id,
				ResponseCode:    code,
				ResponseHeaders: cdp.ToHeaderEntries(headers),
				Body:            body,
			})
			if err != nil {
				o.log.Err(err, "[Orchestrator] 执行响应 FulfillRequest 失败", "requestID", id)
				_ = state.interceptor.ContinueResponse(state.ctx, ts.Client, id)
			} else {
				o.log.Debug("[Orchestrator] 响应修改成功", "requestID", id)
			}
		}

	default:
		if isRequest {
			if err := state.interceptor.ContinueRequest(state.ctx, ts.Client, id); err != nil {
				o.log.Err(err, "[Orchestrator] 默认 ContinueRequest 失败", "requestID", id)
			} else {
				o.log.Debug("[Orchestrator] 请求放行成功", "requestID", id)
			}
		} else {
			if err := state.interceptor.ContinueResponse(state.ctx, ts.Client, id); err != nil {
				o.log.Err(err, "[Orchestrator] 默认 ContinueResponse 失败", "requestID", id)
			} else {
				o.log.Debug("[Orchestrator] 响应放行成功", "requestID", id)
			}
		}
	}
}

// get 获取指定会话的状态
func (o *Orchestrator) get(id domain.SessionID) (*sessionState, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	s, ok := o.sessions[id]
	return s, ok
}
