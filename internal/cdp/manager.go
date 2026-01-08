package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	logger "cdpnetool/internal/logger"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/rpcc"
)

type workspaceMode int

const (
	workspaceModeAutoFollow workspaceMode = iota
	workspaceModeFixed
)

type Manager struct {
	devtoolsURL       string
	conn              *rpcc.Conn
	client            *cdp.Client
	ctx               context.Context
	cancel            context.CancelFunc
	events            chan model.Event
	pending           chan model.PendingItem
	engine            *rules.Engine
	approvalsMu       sync.Mutex
	approvals         map[string]chan rulespec.Rewrite
	pool              *workerPool
	bodySizeThreshold int64
	processTimeoutMS  int
	log               logger.Logger
	attachMu          sync.Mutex
	currentTarget     model.TargetID
	fixedTarget       model.TargetID
	workspaceStop     chan struct{}
	mode              workspaceMode
	watchersMu        sync.Mutex
	watchers          map[model.TargetID]*targetWatcher
}

type targetWatcher struct {
	id     model.TargetID
	conn   *rpcc.Conn
	client *cdp.Client
	cancel context.CancelFunc
}

// New 创建并返回一个管理器，用于管理CDP连接与拦截流程
func New(devtoolsURL string, events chan model.Event, pending chan model.PendingItem, l logger.Logger) *Manager {
	if l == nil {
		l = logger.NewNoopLogger()
	}
	return &Manager{
		devtoolsURL: devtoolsURL,
		events:      events,
		pending:     pending,
		approvals:   make(map[string]chan rulespec.Rewrite),
		log:         l,
		mode:        workspaceModeAutoFollow,
		watchers:    make(map[model.TargetID]*targetWatcher),
	}
}

// AttachTarget 附着到指定浏览器目标并建立CDP会话
func (m *Manager) AttachTarget(target model.TargetID) error {
	m.attachMu.Lock()
	defer m.attachMu.Unlock()
	m.log.Info("开始附加浏览器目标", "devtools", m.devtoolsURL, "target", string(target))
	if target != "" {
		m.fixedTarget = target
		m.mode = workspaceModeFixed
	} else {
		m.fixedTarget = ""
		m.mode = workspaceModeAutoFollow
	}
	if m.cancel != nil {
		m.cancel()
	}
	if m.conn != nil {
		_ = m.conn.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	sel, err := m.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	if sel == nil {
		m.log.Error("未找到可附加的浏览器目标")
		return fmt.Errorf("no target")
	}
	conn, err := rpcc.DialContext(ctx, sel.WebSocketDebuggerURL)
	if err != nil {
		m.log.Error("连接浏览器 DevTools 失败", "error", err)
		return err
	}
	m.conn = conn
	m.client = cdp.NewClient(conn)
	m.currentTarget = model.TargetID(sel.ID)
	m.log.Info("附加浏览器目标成功", "target", string(m.currentTarget))
	if target == "" {
		m.startWorkspaceWatcher()
	} else {
		m.stopWorkspaceWatcher()
	}
	return nil
}

// Detach 断开当前会话连接并释放资源
func (m *Manager) Detach() error {
	m.attachMu.Lock()
	defer m.attachMu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	if m.pool != nil {
		m.pool.stop()
	}
	m.stopWorkspaceWatcher()
	if m.conn != nil {
		return m.conn.Close()
	}
	return nil
}

// Enable 启用Fetch/Network拦截功能并开始消费事件
func (m *Manager) Enable() error {
	if m.client == nil {
		return fmt.Errorf("not attached")
	}
	m.log.Info("开始启用拦截功能")
	err := m.client.Network.Enable(m.ctx, nil)
	if err != nil {
		return err
	}
	p := "*"
	patterns := []fetch.RequestPattern{
		{URLPattern: &p, RequestStage: fetch.RequestStageRequest},
		{URLPattern: &p, RequestStage: fetch.RequestStageResponse},
	}
	err = m.client.Fetch.Enable(m.ctx, &fetch.EnableArgs{Patterns: patterns})
	if err != nil {
		return err
	}
	// 如果已配置 worker pool 且未启动，现在启动
	if m.pool != nil && m.pool.sem != nil && m.ctx != nil {
		m.pool.start(m.ctx)
	}
	go m.consume()
	m.log.Info("拦截功能启用完成")
	return nil
}

// Disable 停止拦截功能但保留连接
func (m *Manager) Disable() error {
	if m.client == nil {
		return fmt.Errorf("not attached")
	}
	return m.client.Fetch.Disable(m.ctx)
}

// consume 持续接收拦截事件并按并发限制分发处理

// dispatchPaused 根据并发配置调度单次拦截事件处理

func (m *Manager) startWorkspaceWatcher() {
	m.log.Debug("开始工作区轮询", "func", "startWorkspaceWatcher")
	if m.workspaceStop != nil {
		return
	}
	ch := make(chan struct{})
	m.workspaceStop = ch
	go m.workspaceLoop(ch)
}

func (m *Manager) stopWorkspaceWatcher() {
	if m.workspaceStop != nil {
		close(m.workspaceStop)
		m.workspaceStop = nil
	}
	m.stopAllWatchers()
}

func (m *Manager) workspaceLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			m.checkWorkspace()
		}
	}
}

func (m *Manager) checkWorkspace() {
	m.log.Debug("开始工作区轮询", "func", "checkWorkspace")
	if m.devtoolsURL == "" {
		return
	}
	if m.fixedTarget != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	dt := devtool.New(m.devtoolsURL)
	targets, err := dt.List(ctx)
	if err != nil {
		m.log.Debug("工作区轮询获取目标列表失败", "error", err)
		return
	}
	m.refreshWatchers(ctx, targets)
	sel := selectAutoTarget(targets)
	if sel == nil {
		return
	}
	candidate := model.TargetID(sel.ID)
	if candidate == "" {
		return
	}
	if m.currentTarget != "" && string(m.currentTarget) == string(candidate) {
		return
	}
	if err := m.attachAndEnable(candidate, true); err != nil {
		m.log.Error("自动切换浏览器目标失败", "error", err)
	}
}

func (m *Manager) attachAndEnable(target model.TargetID, auto bool) error {
	var err error
	if auto {
		err = m.attachAuto(target)
	} else {
		err = m.AttachTarget(target)
	}
	if err != nil {
		return err
	}
	if err := m.Enable(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) attachAuto(target model.TargetID) error {
	m.attachMu.Lock()
	defer m.attachMu.Unlock()
	m.log.Info("自动附加浏览器目标", "devtools", m.devtoolsURL, "target", string(target))
	if m.cancel != nil {
		m.cancel()
	}
	if m.conn != nil {
		_ = m.conn.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	sel, err := m.resolveTarget(ctx, target)
	if err != nil {
		return err
	}
	if sel == nil {
		m.log.Error("未找到可附加的浏览器目标")
		return fmt.Errorf("no target")
	}
	conn, err := rpcc.DialContext(ctx, sel.WebSocketDebuggerURL)
	if err != nil {
		m.log.Error("连接浏览器 DevTools 失败", "error", err)
		return err
	}
	m.conn = conn
	m.client = cdp.NewClient(conn)
	m.currentTarget = model.TargetID(sel.ID)
	m.log.Info("自动附加浏览器目标成功", "target", string(m.currentTarget))
	return nil
}

// decide 构造规则上下文并进行匹配决策
func (m *Manager) decide(ev *fetch.RequestPausedReply, stage string) *rules.Result {
	if m.engine == nil {
		return nil
	}
	ctx := m.buildRuleContext(ev, stage)
	res := m.engine.Eval(ctx)
	if res == nil {
		return nil
	}
	return res
}

func (m *Manager) buildRuleContext(ev *fetch.RequestPausedReply, stage string) rules.Ctx {
	h := map[string]string{}
	q := map[string]string{}
	ck := map[string]string{}
	var bodyText string
	var ctype string

	if stage == "response" {
		if len(ev.ResponseHeaders) > 0 {
			for i := range ev.ResponseHeaders {
				k := ev.ResponseHeaders[i].Name
				v := ev.ResponseHeaders[i].Value
				h[strings.ToLower(k)] = v
				if strings.EqualFold(k, "set-cookie") {
					name, val := parseSetCookie(v)
					if name != "" {
						ck[strings.ToLower(name)] = val
					}
				}
				if strings.EqualFold(k, "content-type") {
					ctype = v
				}
			}
		}
		var clen int64
		if v, ok := h["content-length"]; ok {
			if n, err := parseInt64(v); err == nil {
				clen = n
			}
		}
		if shouldGetBody(ctype, clen, m.bodySizeThreshold) {
			ctx2, cancel := context.WithTimeout(m.ctx, 500*time.Millisecond)
			defer cancel()
			rb, err := m.client.Fetch.GetResponseBody(ctx2, &fetch.GetResponseBodyArgs{RequestID: ev.RequestID})
			if err == nil && rb != nil {
				if rb.Base64Encoded {
					if b, err := base64.StdEncoding.DecodeString(rb.Body); err == nil {
						bodyText = string(b)
					}
				} else {
					bodyText = rb.Body
				}
			}
		}
	} else {
		_ = json.Unmarshal(ev.Request.Headers, &h)
		if len(h) > 0 {
			m2 := make(map[string]string, len(h))
			for k, v := range h {
				m2[strings.ToLower(k)] = v
			}
			h = m2
		}
		if ev.Request.URL != "" {
			if u, err := url.Parse(ev.Request.URL); err == nil {
				for key, vals := range u.Query() {
					if len(vals) > 0 {
						q[strings.ToLower(key)] = vals[0]
					}
				}
			}
		}
		if v, ok := h["cookie"]; ok {
			for name, val := range parseCookie(v) {
				ck[strings.ToLower(name)] = val
			}
		}
		if v, ok := h["content-type"]; ok {
			ctype = v
		}
		if ev.Request.PostData != nil {
			bodyText = *ev.Request.PostData
		}
	}

	return rules.Ctx{URL: ev.Request.URL, Method: ev.Request.Method, Headers: h, Query: q, Cookies: ck, Body: bodyText, ContentType: ctype, Stage: stage}
}

func (m *Manager) resolveTarget(ctx context.Context, target model.TargetID) (*devtool.Target, error) {
	dt := devtool.New(m.devtoolsURL)
	targets, err := dt.List(ctx)
	if err != nil {
		m.log.Error("获取浏览器目标列表失败", "error", err)
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
	return selectAutoTarget(targets), nil
}

func selectAutoTarget(targets []*devtool.Target) *devtool.Target {
	var sel *devtool.Target
	for i := len(targets) - 1; i >= 0; i-- {
		if targets[i].Type != "page" {
			continue
		}
		if !isUserPageURL(targets[i].URL) {
			continue
		}
		sel = targets[i]
		break
	}
	if sel == nil && len(targets) > 0 {
		return targets[0]
	}
	return sel
}

func (m *Manager) refreshWatchers(ctx context.Context, targets []*devtool.Target) {
	ids := make(map[model.TargetID]*devtool.Target)
	for i := range targets {
		if targets[i] == nil {
			continue
		}
		if targets[i].Type != "page" {
			continue
		}
		if !isUserPageURL(targets[i].URL) {
			continue
		}
		id := model.TargetID(targets[i].ID)
		if id == "" {
			continue
		}
		ids[id] = targets[i]
	}
	m.watchersMu.Lock()
	for id, w := range m.watchers {
		if _, ok := ids[id]; !ok {
			w.cancel()
			if w.conn != nil {
				_ = w.conn.Close()
			}
			delete(m.watchers, id)
		}
	}
	for id, t := range ids {
		if _, ok := m.watchers[id]; ok {
			continue
		}
		w, err := m.startWatcher(ctx, id, t.WebSocketDebuggerURL)
		if err != nil {
			m.log.Debug("创建目标可见性监听器失败", "target", string(id), "error", err)
			continue
		}
		m.watchers[id] = w
	}
	m.watchersMu.Unlock()
}

func (m *Manager) startWatcher(ctx context.Context, id model.TargetID, wsURL string) (*targetWatcher, error) {
	wctx, cancel := context.WithCancel(context.Background())
	conn, err := rpcc.DialContext(wctx, wsURL)
	if err != nil {
		cancel()
		return nil, err
	}
	client := cdp.NewClient(conn)
	if err := client.Page.Enable(wctx); err != nil {
		cancel()
		_ = conn.Close()
		return nil, err
	}
	stream, err := client.Page.LifecycleEvent(wctx)
	if err != nil {
		cancel()
		_ = conn.Close()
		return nil, err
	}
	w := &targetWatcher{id: id, conn: conn, client: client, cancel: cancel}
	go func() {
		defer stream.Close()
		for {
			ev, err := stream.Recv()
			if err != nil {
				break
			}
			if ev == nil {
				continue
			}
			name := ev.Name
			if name == "visible" {
				m.onTargetVisible(id)
			}
		}
		m.removeWatcher(id)
	}()
	return w, nil
}

func (m *Manager) onTargetVisible(id model.TargetID) {
	if id == "" {
		return
	}
	if m.mode != workspaceModeAutoFollow {
		return
	}
	if m.currentTarget != "" && m.currentTarget == id {
		return
	}
	if err := m.attachAndEnable(id, true); err != nil {
		m.log.Error("根据可见性切换浏览器目标失败", "target", string(id), "error", err)
	}
}

func (m *Manager) removeWatcher(id model.TargetID) {
	m.watchersMu.Lock()
	defer m.watchersMu.Unlock()
	if w, ok := m.watchers[id]; ok {
		w.cancel()
		if w.conn != nil {
			_ = w.conn.Close()
		}
		delete(m.watchers, id)
	}
}

func (m *Manager) stopAllWatchers() {
	m.watchersMu.Lock()
	defer m.watchersMu.Unlock()
	for id, w := range m.watchers {
		w.cancel()
		if w.conn != nil {
			_ = w.conn.Close()
		}
		delete(m.watchers, id)
	}
}

func (m *Manager) ListTargets(ctx context.Context) ([]model.TargetInfo, error) {
	if m.devtoolsURL == "" {
		return nil, fmt.Errorf("devtools url empty")
	}
	dt := devtool.New(m.devtoolsURL)
	targets, err := dt.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.TargetInfo, 0, len(targets))
	for i := range targets {
		if targets[i] == nil {
			continue
		}
		id := model.TargetID(targets[i].ID)
		info := model.TargetInfo{
			ID:        id,
			Type:      string(targets[i].Type),
			URL:       targets[i].URL,
			Title:     targets[i].Title,
			IsCurrent: m.currentTarget != "" && id == m.currentTarget,
			IsUser:    isUserPageURL(targets[i].URL),
		}
		out = append(out, info)
	}
	return out, nil
}

// SetRules 设置新的规则集并初始化引擎
func (m *Manager) SetRules(rs rulespec.RuleSet) { m.engine = rules.New(rs) }

// UpdateRules 更新已有规则集到引擎
func (m *Manager) UpdateRules(rs rulespec.RuleSet) {
	if m.engine == nil {
		m.engine = rules.New(rs)
	} else {
		m.engine.Update(rs)
	}
}

// Approve 根据审批ID应用外部提供的重写变更
func (m *Manager) Approve(itemID string, mutations rulespec.Rewrite) {
	m.approvalsMu.Lock()
	ch, ok := m.approvals[itemID]
	m.approvalsMu.Unlock()
	if ok {
		select {
		case ch <- mutations:
		default:
		}
	}
}

// SetConcurrency 配置拦截处理的并发工作协程数
func (m *Manager) SetConcurrency(n int) {
	m.pool = newWorkerPool(n)
	if m.pool != nil && m.pool.sem != nil {
		m.pool.setLogger(m.log)
		if m.ctx != nil {
			m.pool.start(m.ctx)
		}
		m.log.Info("并发工作池已启动", "workers", n, "queueCap", m.pool.queueCap)
	} else {
		m.log.Info("并发工作池未限制，使用无界模式")
	}
}

// SetRuntime 设置运行时阈值与处理超时时间
func (m *Manager) SetRuntime(bodySizeThreshold int64, processTimeoutMS int) {
	m.bodySizeThreshold = bodySizeThreshold
	m.processTimeoutMS = processTimeoutMS
}

// GetStats 返回规则引擎的命中统计信息
func (m *Manager) GetStats() model.EngineStats {
	if m.engine == nil {
		return model.EngineStats{ByRule: make(map[model.RuleID]int64)}
	}
	return m.engine.Stats()
}

// GetPoolStats 返回并发工作池的运行统计
func (m *Manager) GetPoolStats() (queueLen, queueCap, totalSubmit, totalDrop int64) {
	if m.pool == nil {
		return 0, 0, 0, 0
	}
	return m.pool.stats()
}
