package manager

import (
	"context"
	"fmt"
	"sync"

	"cdpnetool/internal/logger"
	"cdpnetool/pkg/domain"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/rpcc"
)

// Manager 负责管理浏览器目标会话
type Manager struct {
	devtoolsURL string
	log         logger.Logger
	mu          sync.Mutex
	targets     map[domain.TargetID]*Session
}

// Session 表示一个已附加的浏览器目标会话
type Session struct {
	ID     domain.TargetID
	Conn   *rpcc.Conn
	Client *cdp.Client
	Ctx    context.Context
	Cancel context.CancelFunc
}

// New 创建会话管理器
func New(devtoolsURL string, log logger.Logger) *Manager {
	if log == nil {
		log = logger.NewNop()
	}
	return &Manager{
		devtoolsURL: devtoolsURL,
		log:         log,
		targets:     make(map[domain.TargetID]*Session),
	}
}

// AttachTarget 附加到指定浏览器目标并建立 CDP 会话
func (m *Manager) AttachTarget(target domain.TargetID) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.devtoolsURL == "" {
		return nil, fmt.Errorf("devtools url empty")
	}

	// 已附加则幂等返回
	if target != "" {
		if ts, ok := m.targets[target]; ok {
			return ts, nil
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	selected, err := m.selectTarget(ctx, target)
	if err != nil {
		cancel()
		return nil, err
	}
	if selected == nil {
		cancel()
		m.log.Error("未找到可附加的浏览器目标")
		return nil, fmt.Errorf("no target")
	}

	conn, err := rpcc.DialContext(ctx, selected.WebSocketDebuggerURL)
	if err != nil {
		cancel()
		m.log.Err(err, "连接浏览器 DevTools 失败")
		return nil, err
	}

	client := cdp.NewClient(conn)
	session := &Session{
		ID:     domain.TargetID(selected.ID),
		Conn:   conn,
		Client: client,
		Ctx:    ctx,
		Cancel: cancel,
	}

	m.targets[session.ID] = session
	m.log.Info("附加浏览器目标成功", "target", string(session.ID))

	return session, nil
}

// Detach 断开单个目标连接并释放资源
func (m *Manager) Detach(target domain.TargetID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.targets[target]
	if !ok {
		return nil
	}
	m.closeSession(session)
	delete(m.targets, target)
	return nil
}

// DetachAll 断开所有目标连接并释放资源
func (m *Manager) DetachAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, session := range m.targets {
		m.closeSession(session)
		delete(m.targets, id)
	}
	return nil
}

// GetSession 获取指定目标的会话
func (m *Manager) GetSession(target domain.TargetID) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.targets[target]
	return session, ok
}

// GetAllSessions 获取所有会话
func (m *Manager) GetAllSessions() map[domain.TargetID]*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 返回副本，避免外部修改
	sessions := make(map[domain.TargetID]*Session, len(m.targets))
	for id, session := range m.targets {
		sessions[id] = session
	}
	return sessions
}

// ListTargets 列出当前浏览器中的所有 page 目标，并标记哪些已附加
func (m *Manager) ListTargets(ctx context.Context) ([]domain.TargetInfo, error) {
	if m.devtoolsURL == "" {
		return nil, fmt.Errorf("devtools url empty")
	}

	dt := devtool.New(m.devtoolsURL)
	targets, err := dt.List(ctx)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]domain.TargetInfo, 0, len(targets))
	for i := range targets {
		if targets[i] == nil {
			continue
		}
		if targets[i].Type != "page" {
			continue
		}
		id := domain.TargetID(targets[i].ID)
		info := domain.TargetInfo{
			ID:        id,
			Type:      string(targets[i].Type),
			URL:       targets[i].URL,
			Title:     targets[i].Title,
			IsCurrent: m.targets[id] != nil,
		}
		out = append(out, info)
	}
	return out, nil
}

// closeSession 关闭单个会话
func (m *Manager) closeSession(session *Session) {
	if session == nil {
		return
	}
	if session.Cancel != nil {
		session.Cancel()
	}
	if session.Conn != nil {
		_ = session.Conn.Close()
	}
}

// selectTarget 根据传入的 targetID 或默认策略选择目标
func (m *Manager) selectTarget(ctx context.Context, target domain.TargetID) (*devtool.Target, error) {
	dt := devtool.New(m.devtoolsURL)
	targets, err := dt.List(ctx)
	if err != nil {
		m.log.Err(err, "获取浏览器目标列表失败")
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}

	if target != "" {
		for i := range targets {
			if string(targets[i].ID) == string(target) {
				return targets[i], nil
			}
		}
		return nil, nil
	}

	// 默认选择第一个 page 目标
	for i := range targets {
		if targets[i] == nil {
			continue
		}
		if targets[i].Type != "page" {
			continue
		}
		return targets[i], nil
	}

	return nil, nil
}
