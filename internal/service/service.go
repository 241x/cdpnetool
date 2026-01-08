package service

import (
	"errors"
	"sync"

	"cdpnetool/internal/cdp"
	logger "cdpnetool/internal/logger"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"

	"github.com/google/uuid"
)

type svc struct {
	mu       sync.Mutex
	sessions map[model.SessionID]*session
	log      logger.Logger
}

type session struct {
	id      model.SessionID
	cfg     model.SessionConfig
	rules   rulespec.RuleSet
	events  chan model.Event
	pending chan any
	mgr     *cdp.Manager
}

// New 创建并返回服务层实例
func New() *svc { return NewWithLogger(logger.NewDefaultLogger(logger.LogLevelInfo, nil)) }

func NewWithLogger(l logger.Logger) *svc {
	if l == nil {
		l = logger.NewNoopLogger()
	}
	return &svc{sessions: make(map[model.SessionID]*session), log: l}
}

// StartSession 创建新会话并初始化管理器
func (s *svc) StartSession(cfg model.SessionConfig) (model.SessionID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := model.SessionID(uuid.New().String())
	ses := &session{
		id:      id,
		cfg:     cfg,
		events:  make(chan model.Event, 128),
		pending: make(chan any, cfg.PendingCapacity),
	}
	ses.mgr = cdp.New(cfg.DevToolsURL, ses.events, ses.pending, s.log)
	ses.mgr.SetConcurrency(cfg.Concurrency)
	ses.mgr.SetRuntime(cfg.BodySizeThreshold, cfg.ProcessTimeoutMS)
	s.sessions[id] = ses
	s.log.Info("创建会话成功", "session", string(id), "devtools", cfg.DevToolsURL, "concurrency", cfg.Concurrency, "pending", cfg.PendingCapacity)
	return id, nil
}

// StopSession 停止并清理指定会话
func (s *svc) StopSession(id model.SessionID) error {
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
		_ = ses.mgr.Disable()
		_ = ses.mgr.Detach()
	}
	close(ses.events)
	close(ses.pending)
	s.log.Info("会话已停止", "session", string(id))
	return nil
}

// AttachTarget 为指定会话附着到浏览器目标
func (s *svc) AttachTarget(id model.SessionID, target model.TargetID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr == nil {
		ses.mgr = cdp.New(ses.cfg.DevToolsURL, ses.events, ses.pending, s.log)
		ses.mgr.SetConcurrency(ses.cfg.Concurrency)
		ses.mgr.SetRuntime(ses.cfg.BodySizeThreshold, ses.cfg.ProcessTimeoutMS)
	}
	err := ses.mgr.AttachTarget(target)
	if err == nil {
		s.log.Info("附加浏览器目标成功", "session", string(id), "target", string(target))
	} else {
		s.log.Error("附加浏览器目标失败", "session", string(id), "error", err)
	}
	return err
}

// DetachTarget 为指定会话断开目标连接
func (s *svc) DetachTarget(id model.SessionID, target model.TargetID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr != nil {
		return ses.mgr.Detach()
	}
	return nil
}

// EnableInterception 启用会话的拦截功能
func (s *svc) EnableInterception(id model.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}
	err := ses.mgr.Enable()
	if err == nil {
		s.log.Info("启用会话拦截成功", "session", string(id))
	} else {
		s.log.Error("启用会话拦截失败", "session", string(id), "error", err)
	}
	return err
}

// DisableInterception 停用会话的拦截功能
func (s *svc) DisableInterception(id model.SessionID) error {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	if ses.mgr == nil {
		return errors.New("cdpnetool: manager not initialized")
	}
	err := ses.mgr.Disable()
	if err == nil {
		s.log.Info("停用会话拦截成功", "session", string(id))
	} else {
		s.log.Error("停用会话拦截失败", "session", string(id), "error", err)
	}
	return err
}

// LoadRules 为会话加载规则集并应用到管理器
func (s *svc) LoadRules(id model.SessionID, rs rulespec.RuleSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ses, ok := s.sessions[id]
	if !ok {
		return errors.New("cdpnetool: session not found")
	}
	ses.rules = rs
	s.log.Info("加载规则集完成", "session", string(id), "count", len(rs.Rules), "version", rs.Version)
	if ses.mgr != nil {
		ses.mgr.UpdateRules(rs)
	}
	return nil
}

// GetRuleStats 返回会话内规则引擎的命中统计
func (s *svc) GetRuleStats(id model.SessionID) (model.EngineStats, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return model.EngineStats{ByRule: make(map[model.RuleID]int64)}, nil
	}
	if ses.mgr == nil {
		return model.EngineStats{ByRule: make(map[model.RuleID]int64)}, nil
	}
	return ses.mgr.GetStats(), nil
}

// SubscribeEvents 订阅会话事件流
func (s *svc) SubscribeEvents(id model.SessionID) (<-chan model.Event, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("cdpnetool: session not found")
	}
	return ses.events, nil
}

// SubscribePending 订阅会话的待审批队列
func (s *svc) SubscribePending(id model.SessionID) (<-chan any, error) {
	s.mu.Lock()
	ses, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("cdpnetool: session not found")
	}
	return ses.pending, nil
}

// ApproveRequest 审批请求阶段并应用重写
func (s *svc) ApproveRequest(itemID string, mutations rulespec.Rewrite) error {
	s.mu.Lock()
	for _, ses := range s.sessions {
		if ses.mgr != nil {
			ses.mgr.Approve(itemID, mutations)
		}
	}
	s.mu.Unlock()
	return nil
}

// ApproveResponse 审批响应阶段并应用重写
func (s *svc) ApproveResponse(itemID string, mutations rulespec.Rewrite) error {
	s.mu.Lock()
	for _, ses := range s.sessions {
		if ses.mgr != nil {
			ses.mgr.Approve(itemID, mutations)
		}
	}
	s.mu.Unlock()
	return nil
}

// Reject 拒绝审批项（占位实现）
func (s *svc) Reject(itemID string) error {
	return nil
}
