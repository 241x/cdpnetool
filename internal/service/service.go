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
func (s *svc) StartSession(cfg domain.SessionConfig) (domain.SessionID, error) {
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

	// 拦截器回调：通过 manager 反查 targetID，再交给 handler 处理
	intrHandler := func(client *cdp.Client, ctx context.Context, ev *fetch.RequestPausedReply) {
		var targetID domain.TargetID
		if mgr != nil {
			for id, sess := range mgr.GetAllSessions() {
				if sess != nil && sess.Client == client {
					targetID = id
					break
				}
			}
		}
		h.Handle(client, ctx, targetID, ev)
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
		mgr:      mgr,
		intr:     intr,
		h:        h,
		engine:   nil,
		workPool: workPool,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 探活 DevTools
	_, err := mgr.ListTargets(ctx)
	if err != nil {
		s.log.Err(err, "连接 DevTools 失败", "devtools", cfg.DevToolsURL)
		return "", fmt.Errorf("无法连接到 DevTools: %w", err)
	}

	s.sessions[id] = ses
	s.log.Info("创建会话成功", "session", string(id), "devtools", cfg.DevToolsURL,
		"concurrency", cfg.Concurrency, "pending", cfg.PendingCapacity)
	return id, nil
}

// StopSession 停止并清理指定会话
func (s *svc) StopSession(id domain.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr != nil {
		// 停用拦截并分离所有目标
		if ses.intr != nil {
			sessions := ses.mgr.GetAllSessions()
			for _, ms := range sessions {
				_ = ses.intr.DisableTarget(ms.Client, ms.Ctx)
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
func (s *svc) AttachTarget(id domain.SessionID, target domain.TargetID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}

	if ses.mgr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}

	// 附加目标
	ms, err := ses.mgr.AttachTarget(target)
	if err != nil {
		s.log.Err(err, "附加浏览器目标失败", "session", string(id))
		return err
	}

	// 如果已启用拦截，对新目标立即启用
	if ses.intr != nil && ses.intr.IsEnabled() {
		_ = ses.intr.EnableTarget(ms.Client, ms.Ctx)
	}

	s.log.Info("附加浏览器目标成功", "session", string(id), "target", string(target))
	return nil
}

// DetachTarget 为指定会话断开目标连接
func (s *svc) DetachTarget(id domain.SessionID, target domain.TargetID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr != nil {
		return ses.mgr.Detach(target)
	}
	return nil
}

// ListTargets 列出指定会话中的所有浏览器目标
func (s *svc) ListTargets(id domain.SessionID) ([]domain.TargetInfo, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("cdpnetool: session not found")
	}

	if ses.mgr == nil {
		return nil, errors.New("cdpnetool: manager not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return ses.mgr.ListTargets(ctx)
}

// EnableInterception 启用会话的拦截功能
func (s *svc) EnableInterception(id domain.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr == nil || ses.intr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}

	ses.intr.SetEnabled(true)
	// 为当前所有目标启用拦截
	for _, ms := range ses.mgr.GetAllSessions() {
		if err := ses.intr.EnableTarget(ms.Client, ms.Ctx); err != nil {
			s.log.Err(err, "为目标启用拦截失败", "session", string(id), "target", string(ms.ID))
		}
	}

	s.log.Info("启用会话拦截成功", "session", string(id))
	return nil
}

// DisableInterception 停用会话的拦截功能
func (s *svc) DisableInterception(id domain.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr == nil || ses.intr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}

	ses.intr.SetEnabled(false)
	for _, ms := range ses.mgr.GetAllSessions() {
		if err := ses.intr.DisableTarget(ms.Client, ms.Ctx); err != nil {
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
func (s *svc) LoadRules(id domain.SessionID, cfg *rulespec.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ses, ok := s.sessions[id]
	if !ok {
		return errors.New("cdpnetool: session not found")
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
func (s *svc) GetRuleStats(id domain.SessionID) (domain.EngineStats, error) {
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
func (s *svc) SubscribeEvents(id domain.SessionID) (<-chan domain.NetworkEvent, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("cdpnetool: session not found")
	}
	return ses.events, nil
}
