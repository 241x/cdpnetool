package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"cdpnetool/internal/executor"
	"cdpnetool/internal/handler"
	"cdpnetool/internal/interceptor"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/manager"
	"cdpnetool/internal/pool"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"

	"github.com/google/uuid"
	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/fetch"
)

type svc struct {
	mu       sync.Mutex
	sessions map[domain.SessionID]*session
	log      logger.Logger
}

type session struct {
	id     domain.SessionID
	cfg    domain.SessionConfig
	config *rulespec.Config
	events chan domain.NetworkEvent
	ctx    context.Context    // Session 级上下文
	cancel context.CancelFunc // 用于手动停止 Session

	mgr      *manager.Manager
	intr     *interceptor.Interceptor
	h        *handler.Handler
	engine   *rules.Engine
	workPool *pool.Pool
}

// New 创建并返回服务层实例
func New(l logger.Logger) *svc {
	if l == nil {
		l = logger.NewNop()
	}
	return &svc{sessions: make(map[domain.SessionID]*session), log: l}
}

// StartSession 创建新会话并初始化组件
func (s *svc) StartSession(ctx context.Context, cfg domain.SessionConfig) (domain.SessionID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 32
	}
	if cfg.BodySizeThreshold <= 0 {
		cfg.BodySizeThreshold = 2 << 20 // 2MB
	}
	if cfg.ProcessTimeoutMS <= 0 {
		cfg.ProcessTimeoutMS = 5000
	}
	if cfg.PendingCapacity <= 0 {
		cfg.PendingCapacity = 256
	}

	id := domain.SessionID(uuid.New().String())
	events := make(chan domain.NetworkEvent, cfg.PendingCapacity)

	// 从传入的 ctx (App 级) 派生 Session 级 Context
	sessionCtx, sessionCancel := context.WithCancel(ctx)

	// 会话内组件
	mgr := manager.New(cfg.DevToolsURL, s.log)
	exec := executor.New()
	h := handler.New(handler.Config{
		Engine:           nil,
		Executor:         exec,
		Events:           events,
		ProcessTimeoutMS: cfg.ProcessTimeoutMS,
		Logger:           s.log,
	})

	// 拦截器回调
	intrHandler := func(client *cdp.Client, handlerCtx context.Context, ev *fetch.RequestPausedReply) {
		// 1. 准备基础设施 (TraceID, Logger, Timeout)
		traceID := uuid.New().String()
		l := s.log.With(
			"traceID", traceID,
			"url", ev.Request.URL,
			"requestID", string(ev.RequestID),
		)

		to := cfg.ProcessTimeoutMS
		if to <= 0 {
			to = 3000
		}
		ctx, cancel := context.WithTimeout(handlerCtx, time.Duration(to)*time.Millisecond)
		defer cancel()

		// 2. 获取 TargetID
		var targetID domain.TargetID
		if mgr != nil {
			targetID = mgr.GetTargetIDByClient(client)
		}

		// 3. 根据阶段分发处理
		if ev.ResponseStatusCode == nil {
			h.HandleRequest(client, ctx, targetID, ev, l)
		} else {
			h.HandleResponse(client, ctx, targetID, ev, l)
		}
	}
	intr := interceptor.New(intrHandler, s.log)

	// 并发工作池
	workPool := pool.New(cfg.Concurrency, cfg.PendingCapacity)
	if workPool != nil && workPool.IsEnabled() {
		workPool.SetLogger(s.log)
		intr.SetPool(workPool)
	}

	ses := &session{
		id:       id,
		cfg:      cfg,
		config:   nil,
		events:   events,
		ctx:      sessionCtx,
		cancel:   sessionCancel,
		mgr:      mgr,
		intr:     intr,
		h:        h,
		engine:   nil,
		workPool: workPool,
	}

	// 探活 DevTools
	pingCtx, pingCancel := context.WithTimeout(sessionCtx, 3*time.Second)
	defer pingCancel()

	_, err := mgr.ListTargets(pingCtx)
	if err != nil {
		s.log.Err(err, "连接 DevTools 失败", "devtools", cfg.DevToolsURL)
		sessionCancel()
		return "", fmt.Errorf("%w: %v", domain.ErrDevToolsUnreachable, err)
	}

	s.sessions[id] = ses
	s.log.Info("创建会话成功", "session", string(id), "devtools", cfg.DevToolsURL,
		"concurrency", cfg.Concurrency, "pending", cfg.PendingCapacity)
	return id, nil
}

// StopSession 停止并清理指定会话
func (s *svc) StopSession(ctx context.Context, id domain.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	if !ok {
		return domain.ErrSessionNotFound
	}

	// 立即取消该会话的所有关联 Context
	if ses.cancel != nil {
		ses.cancel()
	}

	if ses.mgr != nil {
		// 停用拦截并分离所有目标 (此处 session 已在关闭中，使用 ctx 级上下文)
		if ses.intr != nil {
			sessions := ses.mgr.GetAttachedTargets()
			for _, ms := range sessions {
				_ = ses.intr.DisableTarget(ms.Client, ctx)
			}
			if ses.workPool != nil {
				ses.workPool.Stop()
			}
		}
		_ = ses.mgr.DetachAll()
	}
	close(ses.events)
	s.log.Info("会话已停止", "session", string(id))
	return nil
}

// AttachTarget 为指定会话附着到浏览器目标
func (s *svc) AttachTarget(ctx context.Context, id domain.SessionID, target domain.TargetID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return domain.ErrSessionNotFound
	}

	if ses.mgr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}

	// 附加目标 (透传 Service 层的 ctx，或者是 Session 的 ctx)
	// 按照计划，这种操作应优先使用传入的 Operation Context (ctx)
	ms, err := ses.mgr.AttachTarget(ctx, target)
	if err != nil {
		s.log.Err(err, "附加浏览器目标失败", "session", string(id))
		return err
	}

	// 如果已启用拦截，对新目标立即启用
	if ses.intr != nil && ses.intr.IsEnabled() {
		_ = ses.intr.EnableTarget(ms.Client, ctx)
	}

	s.log.Info("附加浏览器目标成功", "session", string(id), "target", string(target))
	return nil
}

// DetachTarget 为指定会话断开目标连接
func (s *svc) DetachTarget(ctx context.Context, id domain.SessionID, target domain.TargetID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return domain.ErrSessionNotFound
	}
	if ses.mgr != nil {
		return ses.mgr.Detach(target)
	}
	return nil
}

// ListTargets 列出指定会话中的所有浏览器目标
func (s *svc) ListTargets(ctx context.Context, id domain.SessionID) ([]domain.TargetInfo, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return nil, domain.ErrSessionNotFound
	}

	if ses.mgr == nil {
		return nil, errors.New("cdpnetool: manager not initialized")
	}

	// 使用传入的 ctx 并增加超时保护
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return ses.mgr.ListTargets(queryCtx)
}

// EnableInterception 启用会话的拦截功能
func (s *svc) EnableInterception(ctx context.Context, id domain.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return domain.ErrSessionNotFound
	}
	if ses.mgr == nil || ses.intr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}

	// 业务校验：至少需要附加一个目标
	hasAttached := false
	for _, ms := range ses.mgr.GetAttachedTargets() {
		if ms != nil {
			hasAttached = true
			break
		}
	}
	if !hasAttached {
		return domain.ErrNoTargetAttached
	}

	ses.intr.SetEnabled(true)
	// 为当前所有目标启用拦截 (优先使用当前操作上下文 ctx)
	for _, ms := range ses.mgr.GetAttachedTargets() {
		if err := ses.intr.EnableTarget(ms.Client, ctx); err != nil {
			s.log.Err(err, "为目标启用拦截失败", "session", string(id), "target", string(ms.ID))
		}
	}

	s.log.Info("启用会话拦截成功", "session", string(id))
	return nil
}

// DisableInterception 停用会话的拦截功能
func (s *svc) DisableInterception(ctx context.Context, id domain.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return domain.ErrSessionNotFound
	}
	if ses.mgr == nil || ses.intr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}

	ses.intr.SetEnabled(false)
	for _, ms := range ses.mgr.GetAttachedTargets() {
		if err := ses.intr.DisableTarget(ms.Client, ctx); err != nil {
			s.log.Err(err, "停用目标拦截失败", "session", string(id), "target", string(ms.ID))
		}
	}
	if ses.workPool != nil {
		ses.workPool.Stop()
	}

	s.log.Info("停用会话拦截成功", "session", string(id))
	return nil
}

// LoadRules 为会话加载规则配置并应用到管理器
func (s *svc) LoadRules(ctx context.Context, id domain.SessionID, cfg *rulespec.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ses, ok := s.sessions[id]
	if !ok {
		return domain.ErrSessionNotFound
	}
	ses.config = cfg
	s.log.Info("加载规则配置完成", "session", string(id), "count", len(cfg.Rules), "version", cfg.Version)

	if ses.engine == nil {
		ses.engine = rules.New(cfg)
		if ses.h != nil {
			ses.h.SetEngine(ses.engine)
		}
	} else {
		ses.engine.Update(cfg)
	}
	return nil
}

// GetRuleStats 返回会话内规则引擎的命中统计
func (s *svc) GetRuleStats(ctx context.Context, id domain.SessionID) (domain.EngineStats, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return domain.EngineStats{ByRule: make(map[domain.RuleID]int64)}, nil
	}
	if ses.engine == nil {
		return domain.EngineStats{ByRule: make(map[domain.RuleID]int64)}, nil
	}

	stats := ses.engine.GetStats()
	byRule := make(map[domain.RuleID]int64, len(stats.ByRule))
	for k, v := range stats.ByRule {
		byRule[domain.RuleID(k)] = v
	}

	return domain.EngineStats{
		Total:   stats.Total,
		Matched: stats.Matched,
		ByRule:  byRule,
	}, nil
}

// SubscribeEvents 订阅会话事件流
func (s *svc) SubscribeEvents(ctx context.Context, id domain.SessionID) (<-chan domain.NetworkEvent, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return ses.events, nil
}
