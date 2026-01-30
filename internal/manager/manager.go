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
	devtoolsURL     string
	writeBufferSize int
	log             logger.Logger
	mu              sync.RWMutex // 读写锁，支持并发读
	targets         map[domain.TargetID]*Session
	clientToTarget  map[*cdp.Client]domain.TargetID
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
		devtoolsURL:     devtoolsURL,
		writeBufferSize: 16 * 1024 * 1024,
		log:             log,
		targets:         make(map[domain.TargetID]*Session),
		clientToTarget:  make(map[*cdp.Client]domain.TargetID),
	}
}

// AttachTarget 附加到指定浏览器目标并建立 CDP 会话
func (m *Manager) AttachTarget(ctx context.Context, target domain.TargetID) (*Session, error) {
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

	// 派生 Session 级 Context，用于该目标的整个生命周期
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	selected, err := m.selectTarget(sessionCtx, target)
	if err != nil {
		sessionCancel()
		return nil, err
	}
	if selected == nil {
		sessionCancel()
		m.log.Error("未找到可附加的浏览器目标")
		return nil, fmt.Errorf("no target")
	}

	conn, err := rpcc.DialContext(sessionCtx, selected.WebSocketDebuggerURL,
		rpcc.WithWriteBufferSize(m.writeBufferSize),
		rpcc.WithCompression())
	if err != nil {
		sessionCancel()
		m.log.Err(err, "连接浏览器 DevTools 失败")
		return nil, err
	}

	client := cdp.NewClient(conn)
	session := &Session{
		ID:     domain.TargetID(selected.ID),
		Conn:   conn,
		Client: client,
		Ctx:    sessionCtx,
		Cancel: sessionCancel,
	}

	m.targets[session.ID] = session
	m.clientToTarget[client] = session.ID
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
	delete(m.clientToTarget, session.Client)
	return nil
}

// DetachAll 断开所有目标连接并释放资源
func (m *Manager) DetachAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, session := range m.targets {
		m.closeSession(session)
		delete(m.targets, id)
		delete(m.clientToTarget, session.Client)
	}
	return nil
}

// GetSession 获取指定目标的会话
func (m *Manager) GetSession(target domain.TargetID) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.targets[target]
	return session, ok
}

// GetAttachedTargets 获取所有已附加的目标会话（返回副本）
func (m *Manager) GetAttachedTargets() map[domain.TargetID]*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make(map[domain.TargetID]*Session, len(m.targets))
	for id, session := range m.targets {
		sessions[id] = session
	}
	return sessions
}

// GetTargetIDByClient 根据 CDP Client 反查对应的 TargetID（O(1) 查询）
func (m *Manager) GetTargetIDByClient(client *cdp.Client) domain.TargetID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientToTarget[client]
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

	m.mu.RLock()
	defer m.mu.RUnlock()

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
