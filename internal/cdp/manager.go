package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	logger "cdpnetool/internal/logger"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/model"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/network"
	"github.com/mafredri/cdp/rpcc"
)

type Manager struct {
	devtoolsURL       string
	conn              *rpcc.Conn
	client            *cdp.Client
	ctx               context.Context
	cancel            context.CancelFunc
	events            chan model.Event
	pending           chan any
	engine            *rules.Engine
	approvals         map[string]chan model.Rewrite
	workers           int
	bodySizeThreshold int64
	processTimeoutMS  int
	log               logger.Logger
}

// New 创建并返回一个管理器，用于管理CDP连接与拦截流程
func New(devtoolsURL string, events chan model.Event, pending chan any, l logger.Logger) *Manager {
	return &Manager{devtoolsURL: devtoolsURL, events: events, pending: pending, approvals: make(map[string]chan model.Rewrite), log: l}
}

// AttachTarget 附着到指定浏览器目标并建立CDP会话
func (m *Manager) AttachTarget(target model.TargetID) error {
	if m.log != nil {
		m.log.Info("attach_target_begin", "devtools", m.devtoolsURL, "target", string(target))
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	dt := devtool.New(m.devtoolsURL)
	targets, err := dt.List(ctx)
	if err != nil {
		if m.log != nil {
			m.log.Error("attach_target_list_error", "error", err)
		}
		return err
	}
	var sel *devtool.Target
	for i := range targets {
		if string(targets[i].ID) == string(target) || target == "" {
			sel = targets[i]
			if target == "" {
				break
			}
		}
	}
	if sel == nil {
		if m.log != nil {
			m.log.Error("attach_target_none")
		}
		return fmt.Errorf("no target")
	}
	conn, err := rpcc.DialContext(ctx, sel.WebSocketDebuggerURL)
	if err != nil {
		if m.log != nil {
			m.log.Error("attach_target_dial_error", "error", err)
		}
		return err
	}
	m.conn = conn
	m.client = cdp.NewClient(conn)
	if m.log != nil {
		m.log.Info("attach_target_success")
	}
	return nil
}

// Detach 断开当前会话连接并释放资源
func (m *Manager) Detach() error {
	if m.cancel != nil {
		m.cancel()
	}
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
	if m.log != nil {
		m.log.Info("enable_begin")
	}
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
	go m.consume()
	if m.log != nil {
		m.log.Info("enable_done", "workers", m.workers)
	}
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
func (m *Manager) consume() {
	rp, err := m.client.Fetch.RequestPaused(m.ctx)
	if err != nil {
		if m.log != nil {
			m.log.Error("consume_subscribe_error", "error", err)
		}
		return
	}
	defer rp.Close()
	var sem chan struct{}
	if m.workers > 0 {
		sem = make(chan struct{}, m.workers)
	}
	if m.log != nil {
		m.log.Info("consume_start")
	}
	for {
		ev, err := rp.Recv()
		if err != nil {
			if m.log != nil {
				m.log.Error("consume_recv_error", "error", err)
			}
			return
		}
		if sem != nil {
			sem <- struct{}{}
			go func(e *fetch.RequestPausedReply) {
				defer func() { <-sem }()
				m.handle(e)
			}(ev)
		} else {
			go m.handle(ev)
		}
	}
}

// handle 处理一次拦截事件并根据规则执行相应动作
func (m *Manager) handle(ev *fetch.RequestPausedReply) {
	to := m.processTimeoutMS
	if to <= 0 {
		to = 3000
	}
	ctx, cancel := context.WithTimeout(m.ctx, time.Duration(to)*time.Millisecond)
	defer cancel()
	start := time.Now()
	m.events <- model.Event{Type: "intercepted"}
	stg := "request"
	if ev.ResponseStatusCode != nil {
		stg = "response"
	}
	if m.log != nil {
		m.log.Debug("handle_start", "stage", stg, "url", ev.Request.URL, "method", ev.Request.Method)
	}
	res := m.decide(ev, stg)
	if res == nil || res.Action == nil {
		m.applyContinue(ctx, ev, stg)
		return
	}
	a := res.Action
	if a.DropRate > 0 {
		if rand.Float64() < a.DropRate {
			m.applyContinue(ctx, ev, stg)
			m.events <- model.Event{Type: "degraded"}
			if m.log != nil {
				m.log.Warn("drop_rate_triggered", "stage", stg)
			}
			return
		}
	}
	if a.DelayMS > 0 {
		time.Sleep(time.Duration(a.DelayMS) * time.Millisecond)
	}
	if time.Since(start) > time.Duration(to)*time.Millisecond {
		m.applyContinue(ctx, ev, stg)
		m.events <- model.Event{Type: "degraded"}
		if m.log != nil {
			m.log.Warn("process_timeout", "stage", stg)
		}
		return
	}
	if a.Pause != nil {
		if m.log != nil {
			m.log.Info("apply_pause", "stage", stg)
		}
		m.applyPause(ctx, ev, a.Pause, stg)
		return
	}
	if a.Fail != nil {
		if m.log != nil {
			m.log.Info("apply_fail", "stage", stg)
		}
		m.applyFail(ctx, ev, a.Fail)
		m.events <- model.Event{Type: "failed", Rule: res.RuleID}
		return
	}
	if a.Respond != nil {
		if m.log != nil {
			m.log.Info("apply_respond", "stage", stg)
		}
		m.applyRespond(ctx, ev, a.Respond, stg)
		m.events <- model.Event{Type: "fulfilled", Rule: res.RuleID}
		return
	}
	if a.Rewrite != nil {
		if m.log != nil {
			m.log.Info("apply_rewrite", "stage", stg)
		}
		m.applyRewrite(ctx, ev, a.Rewrite, stg)
		m.events <- model.Event{Type: "mutated", Rule: res.RuleID}
		return
	}
	m.applyContinue(ctx, ev, stg)
}

// decide 构造规则上下文并进行匹配决策
func (m *Manager) decide(ev *fetch.RequestPausedReply, stage string) *rules.Result {
	if m.engine == nil {
		return nil
	}
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
	res := m.engine.Eval(rules.Ctx{URL: ev.Request.URL, Method: ev.Request.Method, Headers: h, Query: q, Cookies: ck, Body: bodyText, ContentType: ctype, Stage: stage})
	if res == nil {
		return nil
	}
	return res
}

// parseCookie 解析Cookie头为键值对映射
func parseCookie(s string) map[string]string {
	out := make(map[string]string)
	parts := strings.Split(s, ";")
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

// parseSetCookie 解析Set-Cookie的首个键值
func parseSetCookie(s string) (string, string) {
	// CookieName=CookieValue; Attr=...
	p := strings.SplitN(s, ";", 2)
	first := strings.TrimSpace(p[0])
	kv := strings.SplitN(first, "=", 2)
	if len(kv) == 2 {
		return kv[0], kv[1]
	}
	return "", ""
}

// urlParse 解析并按补丁修改查询参数后返回URL
func urlParse(raw string, qpatch map[string]*string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range qpatch {
		if v == nil {
			q.Del(k)
		} else {
			q.Set(k, *v)
		}
	}
	u.RawQuery = q.Encode()
	return u, nil
}

// shouldGetBody 判断是否需要获取响应体以用于匹配或重写
func shouldGetBody(ctype string, clen int64, thr int64) bool {
	if thr <= 0 {
		thr = 4 * 1024 * 1024
	}
	if clen > 0 && clen > thr {
		return false
	}
	lc := strings.ToLower(ctype)
	if strings.HasPrefix(lc, "text/") {
		return true
	}
	if strings.HasPrefix(lc, "application/json") {
		return true
	}
	return false
}

// parseInt64 将数字字符串解析为int64
func parseInt64(s string) (int64, error) {
	var n int64
	var mul int64 = 1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid")
		}
		n = n*10 + int64(c-'0')
	}
	return n * mul, nil
}

// applyContinue 继续原请求或响应不做修改
func (m *Manager) applyContinue(ctx context.Context, ev *fetch.RequestPausedReply, stage string) {
	if stage == "response" {
		m.client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: ev.RequestID})
		if m.log != nil {
			m.log.Debug("continue_response")
		}
	} else {
		m.client.Fetch.ContinueRequest(ctx, &fetch.ContinueRequestArgs{RequestID: ev.RequestID})
		if m.log != nil {
			m.log.Debug("continue_request")
		}
	}
}

// applyFail 使请求失败并返回错误原因
func (m *Manager) applyFail(ctx context.Context, ev *fetch.RequestPausedReply, f *model.Fail) {
	m.client.Fetch.FailRequest(ctx, &fetch.FailRequestArgs{RequestID: ev.RequestID, ErrorReason: network.ErrorReasonFailed})
}

// applyRespond 返回自定义响应（可只改头或完整替换）
func (m *Manager) applyRespond(ctx context.Context, ev *fetch.RequestPausedReply, r *model.Respond, stage string) {
	if stage == "response" && len(r.Body) == 0 {
		// 仅修改响应码/头，继续响应
		args := &fetch.ContinueResponseArgs{RequestID: ev.RequestID}
		if r.Status != 0 {
			args.ResponseCode = &r.Status
		}
		if len(r.Headers) > 0 {
			args.ResponseHeaders = toHeaderEntries(r.Headers)
		}
		m.client.Fetch.ContinueResponse(ctx, args)
		return
	}
	// fulfill 完整响应
	args := &fetch.FulfillRequestArgs{RequestID: ev.RequestID, ResponseCode: r.Status}
	if len(r.Headers) > 0 {
		args.ResponseHeaders = toHeaderEntries(r.Headers)
	}
	if len(r.Body) > 0 {
		args.Body = r.Body
	}
	m.client.Fetch.FulfillRequest(ctx, args)
}

// applyRewrite 根据规则对请求或响应进行重写
func (m *Manager) applyRewrite(ctx context.Context, ev *fetch.RequestPausedReply, rw *model.Rewrite, stage string) {
	var url, method *string
	if rw.URL != nil {
		url = rw.URL
	}
	if rw.Method != nil {
		method = rw.Method
	}
	var hdrs []fetch.HeaderEntry
	if rw.Headers != nil {
		for k, v := range rw.Headers {
			if v != nil {
				hdrs = append(hdrs, fetch.HeaderEntry{Name: k, Value: *v})
			}
		}
	}
	if stage == "response" {
		var needBody bool
		if rw.Body != nil {
			needBody = true
		}
		if !needBody {
			if rw.Headers != nil {
				cur := make(map[string]string, len(ev.ResponseHeaders))
				for i := range ev.ResponseHeaders {
					cur[strings.ToLower(ev.ResponseHeaders[i].Name)] = ev.ResponseHeaders[i].Value
				}
				for k, v := range rw.Headers {
					lk := strings.ToLower(k)
					if v == nil {
						delete(cur, lk)
					} else {
						cur[lk] = *v
					}
				}
				var out []fetch.HeaderEntry
				for k, v := range cur {
					out = append(out, fetch.HeaderEntry{Name: k, Value: v})
				}
				m.client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: ev.RequestID, ResponseHeaders: out})
				return
			}
			m.client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: ev.RequestID})
			return
		}
		var ctype string
		var clen int64
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
		if !shouldGetBody(ctype, clen, m.bodySizeThreshold) {
			m.client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: ev.RequestID})
			return
		}
		ctx2, cancel := context.WithTimeout(m.ctx, 500*time.Millisecond)
		defer cancel()
		rb, err := m.client.Fetch.GetResponseBody(ctx2, &fetch.GetResponseBodyArgs{RequestID: ev.RequestID})
		if err != nil || rb == nil {
			m.client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: ev.RequestID})
			return
		}
		var bodyText string
		if rb.Base64Encoded {
			if b, err := base64.StdEncoding.DecodeString(rb.Body); err == nil {
				bodyText = string(b)
			}
		} else {
			bodyText = rb.Body
		}
		var newBody []byte
		switch rw.Body.Type {
		case "base64":
			if len(rw.Body.Ops) > 0 {
				if s, ok := rw.Body.Ops[0].(string); ok {
					if b, err := base64.StdEncoding.DecodeString(s); err == nil {
						newBody = b
					}
				}
			}
		case "text_regex":
			if len(rw.Body.Ops) >= 2 {
				p, pOk := rw.Body.Ops[0].(string)
				r, rOk := rw.Body.Ops[1].(string)
				if pOk && rOk {
					re, err := regexp.Compile(p)
					if err == nil {
						newBody = []byte(re.ReplaceAllString(bodyText, r))
					}
				}
			}
		case "json_patch":
			if out, ok := applyJSONPatch(bodyText, rw.Body.Ops); ok {
				newBody = []byte(out)
			}
		}
		if len(newBody) == 0 {
			m.client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: ev.RequestID})
			return
		}
		code := 200
		if ev.ResponseStatusCode != nil {
			code = *ev.ResponseStatusCode
		}
		args := &fetch.FulfillRequestArgs{RequestID: ev.RequestID, ResponseCode: code}
		cur := make(map[string]string)
		for i := range ev.ResponseHeaders {
			cur[strings.ToLower(ev.ResponseHeaders[i].Name)] = ev.ResponseHeaders[i].Value
		}
		if rw.Headers != nil {
			for k, v := range rw.Headers {
				lk := strings.ToLower(k)
				if v == nil {
					delete(cur, lk)
				} else {
					cur[lk] = *v
				}
			}
		}
		args.ResponseHeaders = toHeaderEntries(cur)
		args.Body = newBody
		m.client.Fetch.FulfillRequest(ctx, args)
		return
	}
	if rw.Cookies != nil {
		h := map[string]string{}
		_ = json.Unmarshal(ev.Request.Headers, &h)
		var cookie string
		for k, v := range h {
			if strings.EqualFold(k, "cookie") {
				cookie = v
				break
			}
		}
		cm := parseCookie(cookie)
		for name, val := range rw.Cookies {
			if val == nil {
				delete(cm, name)
			} else {
				cm[name] = *val
			}
		}
		if len(cm) > 0 {
			var b strings.Builder
			first := true
			for k, v := range cm {
				if !first {
					b.WriteString("; ")
				}
				first = false
				b.WriteString(k)
				b.WriteString("=")
				b.WriteString(v)
			}
			hdrs = append(hdrs, fetch.HeaderEntry{Name: "Cookie", Value: b.String()})
		}
	}
	var post []byte
	if rw.Body != nil {
		switch rw.Body.Type {
		case "base64":
			if len(rw.Body.Ops) > 0 {
				if s, ok := rw.Body.Ops[0].(string); ok {
					b, err := base64.StdEncoding.DecodeString(s)
					if err == nil {
						post = b
					}
				}
			}
		case "text_regex":
			if ev.Request.PostData != nil {
				src := *ev.Request.PostData
				if len(rw.Body.Ops) >= 2 {
					p, pOk := rw.Body.Ops[0].(string)
					r, rOk := rw.Body.Ops[1].(string)
					if pOk && rOk {
						re, err := regexp.Compile(p)
						if err == nil {
							post = []byte(re.ReplaceAllString(src, r))
						}
					}
				}
			}
		case "json_patch":
			var src string
			if ev.Request.PostData != nil {
				src = *ev.Request.PostData
			}
			if out, ok := applyJSONPatch(src, rw.Body.Ops); ok {
				post = []byte(out)
			}
		}
	}
	args := &fetch.ContinueRequestArgs{RequestID: ev.RequestID, URL: url, Method: method, Headers: hdrs}
	if rw.Query != nil && url == nil {
		if u, err := urlParse(ev.Request.URL, rw.Query); err == nil {
			us := u.String()
			args.URL = &us
		}
	}
	if len(post) > 0 {
		args.PostData = post
	}
	m.client.Fetch.ContinueRequest(ctx, args)
}

// applyJSONPatch 对JSON文档应用Patch操作并返回结果
func applyJSONPatch(doc string, ops []any) (string, bool) {
	var v any
	if doc == "" {
		v = make(map[string]any)
	} else {
		if err := json.Unmarshal([]byte(doc), &v); err != nil {
			return "", false
		}
	}
	for _, op := range ops {
		m, ok := op.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["op"].(string)
		path, _ := m["path"].(string)
		val := m["value"]
		from, _ := m["from"].(string)
		switch typ {
		case "add", "replace":
			v = setByPtr(v, path, val, typ == "replace")
		case "remove":
			v = removeByPtr(v, path)
		case "copy":
			src, ok := getByPtr(v, from)
			if !ok {
				return "", false
			}
			v = setByPtr(v, path, src, true)
		case "move":
			src, ok := getByPtr(v, from)
			if !ok {
				return "", false
			}
			v = removeByPtr(v, from)
			v = setByPtr(v, path, src, true)
		case "test":
			cur, ok := getByPtr(v, path)
			if !ok {
				return "", false
			}
			if !deepEqual(cur, val) {
				return "", false
			}
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// setByPtr 依据JSON Pointer设置节点值
func setByPtr(cur any, ptr string, val any, replace bool) any {
	if ptr == "" || ptr[0] != '/' {
		return cur
	}
	tokens := splitPtr(ptr)
	return setRec(cur, tokens, val)
}

// setRec 递归设置节点值的内部实现
func setRec(cur any, tokens []string, val any) any {
	if len(tokens) == 0 {
		return val
	}
	t := tokens[0]
	switch c := cur.(type) {
	case map[string]any:
		child, ok := c[t]
		if !ok {
			child = make(map[string]any)
		}
		c[t] = setRec(child, tokens[1:], val)
		return c
	case []any:
		idx, ok := toIndex(t)
		if !ok || idx < 0 || idx >= len(c) {
			return c
		}
		c[idx] = setRec(c[idx], tokens[1:], val)
		return c
	default:
		if len(tokens) == 1 {
			return val
		}
		return cur
	}
}

// removeByPtr 依据JSON Pointer移除节点
func removeByPtr(cur any, ptr string) any {
	if ptr == "" || ptr[0] != '/' {
		return cur
	}
	tokens := splitPtr(ptr)
	return removeRec(cur, tokens)
}

// getByPtr 依据JSON Pointer读取节点值
func getByPtr(cur any, ptr string) (any, bool) {
	if ptr == "" || ptr[0] != '/' {
		return nil, false
	}
	tokens := splitPtr(ptr)
	x := cur
	for _, t := range tokens {
		switch c := x.(type) {
		case map[string]any:
			v, ok := c[t]
			if !ok {
				return nil, false
			}
			x = v
		case []any:
			idx, ok := toIndex(t)
			if !ok || idx < 0 || idx >= len(c) {
				return nil, false
			}
			x = c[idx]
		default:
			return nil, false
		}
	}
	return x, true
}

// deepEqual 深度比较两个值是否相等
func deepEqual(a, b any) bool { return reflect.DeepEqual(a, b) }

// removeRec 递归移除节点的内部实现
func removeRec(cur any, tokens []string) any {
	if len(tokens) == 0 {
		return cur
	}
	t := tokens[0]
	switch c := cur.(type) {
	case map[string]any:
		if len(tokens) == 1 {
			delete(c, t)
			return c
		}
		child, ok := c[t]
		if !ok {
			return c
		}
		c[t] = removeRec(child, tokens[1:])
		return c
	case []any:
		idx, ok := toIndex(t)
		if !ok || idx < 0 || idx >= len(c) {
			return c
		}
		if len(tokens) == 1 {
			nc := append(c[:idx], c[idx+1:]...)
			return nc
		}
		c[idx] = removeRec(c[idx], tokens[1:])
		return c
	default:
		return cur
	}
}

// splitPtr 将JSON Pointer切分为令牌序列
func splitPtr(p string) []string {
	var out []string
	i := 1
	for i < len(p) {
		j := i
		for j < len(p) && p[j] != '/' {
			j++
		}
		tok := p[i:j]
		tok = strings.ReplaceAll(tok, "~1", "/")
		tok = strings.ReplaceAll(tok, "~0", "~")
		out = append(out, tok)
		i = j + 1
	}
	return out
}

// toIndex 将字符串转换为数组索引
func toIndex(s string) (int, bool) {
	n := 0
	if len(s) == 0 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// toHeaderEntries 将头部映射转换为CDP头部条目
func toHeaderEntries(h map[string]string) []fetch.HeaderEntry {
	out := make([]fetch.HeaderEntry, 0, len(h))
	for k, v := range h {
		out = append(out, fetch.HeaderEntry{Name: k, Value: v})
	}
	return out
}

// applyPause 进入人工审批流程并按超时默认动作处理
func (m *Manager) applyPause(ctx context.Context, ev *fetch.RequestPausedReply, p *model.Pause, stage string) {
	id := string(ev.RequestID)
	ch := make(chan model.Rewrite, 1)
	m.approvals[id] = ch
	if m.pending != nil {
		select {
		case m.pending <- struct{ ID string }{ID: id}:
		default:
			switch p.DefaultAction.Type {
			case "fulfill":
				m.applyRespond(ctx, ev, &model.Respond{Status: p.DefaultAction.Status}, stage)
			case "fail":
				m.applyFail(ctx, ev, &model.Fail{Reason: p.DefaultAction.Reason})
			case "continue_mutated":
				m.applyContinue(ctx, ev, stage)
			default:
				m.applyContinue(ctx, ev, stage)
			}
			m.events <- model.Event{Type: "degraded"}
			delete(m.approvals, id)
			return
		}
	}
	t := time.NewTimer(time.Duration(p.TimeoutMS) * time.Millisecond)
	select {
	case mut := <-ch:
		_ = mut
		m.applyContinue(ctx, ev, stage)
	case <-t.C:
		switch p.DefaultAction.Type {
		case "fulfill":
			m.applyRespond(ctx, ev, &model.Respond{Status: p.DefaultAction.Status}, stage)
		case "fail":
			m.applyFail(ctx, ev, &model.Fail{Reason: p.DefaultAction.Reason})
		case "continue_mutated":
			m.applyContinue(ctx, ev, stage)
		default:
			m.applyContinue(ctx, ev, stage)
		}
	}
	delete(m.approvals, id)
}

// SetRules 设置新的规则集并初始化引擎
func (m *Manager) SetRules(rs model.RuleSet) { m.engine = rules.New(rs) }

// UpdateRules 更新已有规则集到引擎
func (m *Manager) UpdateRules(rs model.RuleSet) {
	if m.engine == nil {
		m.engine = rules.New(rs)
	} else {
		m.engine.Update(rs)
	}
}

// Approve 根据审批ID应用外部提供的重写变更
func (m *Manager) Approve(itemID string, mutations model.Rewrite) {
	if ch, ok := m.approvals[itemID]; ok {
		ch <- mutations
	}
}

// SetConcurrency 配置拦截处理的并发工作协程数
func (m *Manager) SetConcurrency(n int) { m.workers = n }

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
