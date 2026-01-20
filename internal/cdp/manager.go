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

	"cdpnetool/internal/logger"
	"cdpnetool/internal/pool"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/rpcc"
)

// Manager 负责管理一个会话下的所有浏览器 page 目标
type Manager struct {
	devtoolsURL       string
	log               logger.Logger
	engine            *rules.Engine
	executor          *ActionExecutor
	bodySizeThreshold int64
	processTimeoutMS  int
	pool              *pool.Pool
	events            chan model.InterceptEvent
	targetsMu         sync.Mutex
	targets           map[model.TargetID]*targetSession
	stateMu           sync.RWMutex
	enabled           bool
}

// targetSession 表示一个已附加并可拦截的 page 目标
type targetSession struct {
	id     model.TargetID
	conn   *rpcc.Conn
	client *cdp.Client
	ctx    context.Context
	cancel context.CancelFunc
}

// New 创建并返回一个管理器，用于管理 CDP 连接与拦截流程
func New(devtoolsURL string, events chan model.InterceptEvent, l logger.Logger) *Manager {
	if l == nil {
		l = logger.NewNop()
	}
	m := &Manager{
		devtoolsURL: devtoolsURL,
		log:         l,
		events:      events,
		targets:     make(map[model.TargetID]*targetSession),
	}
	m.executor = NewActionExecutor(m)
	return m
}

// setEnabled 设置拦截开关
func (m *Manager) setEnabled(v bool) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.enabled = v
}

// isEnabled 获取当前拦截开关状态
func (m *Manager) isEnabled() bool {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.enabled
}

// AttachTarget 附加到指定浏览器目标并建立 CDP 会话。
func (m *Manager) AttachTarget(target model.TargetID) error {
	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	if m.devtoolsURL == "" {
		return fmt.Errorf("devtools url empty")
	}

	// 已附加则幂等返回
	if target != "" {
		if _, ok := m.targets[target]; ok {
			return nil
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	selected, err := m.selectTarget(ctx, target)
	if err != nil {
		cancel()
		return err
	}
	if selected == nil {
		cancel()
		m.log.Error("未找到可附加的浏览器目标")
		return fmt.Errorf("no target")
	}

	conn, err := rpcc.DialContext(ctx, selected.WebSocketDebuggerURL)
	if err != nil {
		cancel()
		m.log.Err(err, "连接浏览器 DevTools 失败")
		return err
	}

	client := cdp.NewClient(conn)
	ts := &targetSession{
		id:     model.TargetID(selected.ID),
		conn:   conn,
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}

	m.targets[ts.id] = ts
	m.log.Info("附加浏览器目标成功", "target", string(ts.id))

	// 如果会话已经启用拦截，则对新目标立即启用
	if m.isEnabled() {
		if err := m.enableTarget(ts); err != nil {
			m.log.Err(err, "为新目标启用拦截失败", "target", string(ts.id))
		}
	}

	return nil
}

// Detach 断开单个目标连接并释放资源。
func (m *Manager) Detach(target model.TargetID) error {
	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	ts, ok := m.targets[target]
	if !ok {
		return nil
	}
	m.closeTargetSession(ts)
	delete(m.targets, target)
	return nil
}

// DetachAll 断开所有目标连接并释放资源。
func (m *Manager) DetachAll() error {
	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	for id, ts := range m.targets {
		m.closeTargetSession(ts)
		delete(m.targets, id)
	}
	return nil
}

// closeTargetSession 关闭单个 targetSession
func (m *Manager) closeTargetSession(ts *targetSession) {
	if ts == nil {
		return
	}
	if ts.cancel != nil {
		ts.cancel()
	}
	if ts.conn != nil {
		_ = ts.conn.Close()
	}
}

// Enable 启用 Fetch/Network 拦截功能并开始消费事件
func (m *Manager) Enable() error {
	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	if len(m.targets) == 0 {
		return fmt.Errorf("no targets attached")
	}

	m.log.Info("开始启用拦截功能")
	m.setEnabled(true)

	for id, ts := range m.targets {
		if err := m.enableTarget(ts); err != nil {
			m.log.Err(err, "为目标启用拦截失败", "target", string(id))
		}
	}

	m.log.Info("拦截功能启用完成")
	return nil
}

// enableTarget 为单个目标启用 Network/Fetch 并启动事件消费
func (m *Manager) enableTarget(ts *targetSession) error {
	if ts == nil || ts.client == nil {
		return fmt.Errorf("target client not initialized")
	}

	if err := ts.client.Network.Enable(ts.ctx, nil); err != nil {
		return err
	}

	p := "*"
	patterns := []fetch.RequestPattern{
		{URLPattern: &p, RequestStage: fetch.RequestStageRequest},
		{URLPattern: &p, RequestStage: fetch.RequestStageResponse},
	}
	if err := ts.client.Fetch.Enable(ts.ctx, &fetch.EnableArgs{Patterns: patterns}); err != nil {
		return err
	}

	// 如果已配置 worker pool 且未启动，现在启动
	if m.pool != nil && m.pool.IsEnabled() {
		m.pool.SetLogger(m.log)
		m.pool.Start(ts.ctx)
	}

	go m.consume(ts)
	return nil
}

// Disable 停止拦截功能但保留连接
func (m *Manager) Disable() error {
	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	if len(m.targets) == 0 {
		m.setEnabled(false)
		return nil
	}

	m.setEnabled(false)

	for id, ts := range m.targets {
		if ts.client == nil {
			continue
		}
		if err := ts.client.Fetch.Disable(ts.ctx); err != nil {
			m.log.Err(err, "停用目标拦截失败", "target", string(id))
		}
	}

	return nil
}

// buildEvalContext 构造规则匹配上下文
func (m *Manager) buildEvalContext(ev *fetch.RequestPausedReply) *rules.EvalContext {
	h := map[string]string{}
	q := map[string]string{}
	ck := map[string]string{}
	var bodyText string
	var resourceType string

	// 获取资源类型
	if ev.ResourceType != "" {
		resourceType = string(ev.ResourceType)
	}

	// 解析请求头
	_ = json.Unmarshal(ev.Request.Headers, &h)
	if len(h) > 0 {
		m2 := make(map[string]string, len(h))
		for k, v := range h {
			m2[strings.ToLower(k)] = v
		}
		h = m2
	}

	// 解析 Query 参数
	if ev.Request.URL != "" {
		if u, err := url.Parse(ev.Request.URL); err == nil {
			for key, vals := range u.Query() {
				if len(vals) > 0 {
					q[strings.ToLower(key)] = vals[0]
				}
			}
		}
	}

	// 解析 Cookie
	if v, ok := h["cookie"]; ok {
		for name, val := range parseCookie(v) {
			ck[strings.ToLower(name)] = val
		}
	}

	// 获取请求体
	bodyText = GetRequestBody(ev)

	return &rules.EvalContext{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		ResourceType: resourceType,
		Headers:      h,
		Query:        q,
		Cookies:      ck,
		Body:         bodyText,
	}
}

// getResponseBody 获取响应体内容
func (m *Manager) getResponseBody(ts *targetSession, ev *fetch.RequestPausedReply) string {
	var ctype string
	var clen int64

	if len(ev.ResponseHeaders) > 0 {
		for i := range ev.ResponseHeaders {
			k := ev.ResponseHeaders[i].Name
			v := ev.ResponseHeaders[i].Value
			if strings.EqualFold(k, "content-type") {
				ctype = v
			}
			if strings.EqualFold(k, "content-length") {
				if n, err := parseInt64(v); err == nil {
					clen = n
				}
			}
		}
	}

	if !shouldGetBody(ctype, clen, m.bodySizeThreshold) {
		return ""
	}

	ctx2, cancel := context.WithTimeout(ts.ctx, 500*time.Millisecond)
	defer cancel()
	rb, err := ts.client.Fetch.GetResponseBody(ctx2, &fetch.GetResponseBodyArgs{RequestID: ev.RequestID})
	if err != nil || rb == nil {
		return ""
	}

	if rb.Base64Encoded {
		if b, err := base64.StdEncoding.DecodeString(rb.Body); err == nil {
			return string(b)
		}
		return ""
	}
	return rb.Body
}

// selectTarget 根据传入的 targetID 或默认策略选择目标
func (m *Manager) selectTarget(ctx context.Context, target model.TargetID) (*devtool.Target, error) {
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

	// 默认选择第一个 page 目标，不做 URL 过滤
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

// ListTargets 列出当前浏览器中的所有 page 目标，并标记哪些已附加
func (m *Manager) ListTargets(ctx context.Context) ([]model.TargetInfo, error) {
	if m.devtoolsURL == "" {
		return nil, fmt.Errorf("devtools url empty")
	}

	dt := devtool.New(m.devtoolsURL)
	targets, err := dt.List(ctx)
	if err != nil {
		return nil, err
	}

	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	out := make([]model.TargetInfo, 0, len(targets))
	for i := range targets {
		if targets[i] == nil {
			continue
		}
		if targets[i].Type != "page" {
			continue
		}
		id := model.TargetID(targets[i].ID)
		info := model.TargetInfo{
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

// SetRules 设置新的规则配置并初始化引擎
func (m *Manager) SetRules(cfg *rulespec.Config) {
	m.engine = rules.New(cfg)
}

// UpdateRules 更新已有规则配置到引擎
func (m *Manager) UpdateRules(cfg *rulespec.Config) {
	if m.engine == nil {
		m.engine = rules.New(cfg)
	} else {
		m.engine.Update(cfg)
	}
}

// SetConcurrency 配置拦截处理的并发工作协程数和队列容量
func (m *Manager) SetConcurrency(n int, queueCap int) {
	m.pool = pool.New(n, queueCap)
	if m.pool != nil && m.pool.IsEnabled() {
		m.pool.SetLogger(m.log)
		m.log.Info("并发工作池已配置", "workers", n, "queueCap", m.pool.GetQueueCap())
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

	stats := m.engine.GetStats()
	byRule := make(map[model.RuleID]int64, len(stats.ByRule))
	for k, v := range stats.ByRule {
		byRule[model.RuleID(k)] = v
	}

	return model.EngineStats{
		Total:   stats.Total,
		Matched: stats.Matched,
		ByRule:  byRule,
	}
}

// GetPoolStats 返回并发工作池的运行统计
func (m *Manager) GetPoolStats() (queueLen, queueCap, totalSubmit, totalDrop int64) {
	if m.pool == nil {
		return 0, 0, 0, 0
	}
	return m.pool.Stats()
}
