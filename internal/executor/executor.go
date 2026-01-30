package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cdpnetool/internal/logger"
	"cdpnetool/internal/protocol"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/tidwall/sjson"
)

// RequestMutation 请求修改结果
type RequestMutation struct {
	URL           *string
	Method        *string
	Headers       map[string]string
	RemoveHeaders []string
	Query         map[string]string
	RemoveQuery   []string
	Cookies       map[string]string
	RemoveCookies []string
	Body          *string
}

// BlockResponse 拦截响应
type BlockResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// ResponseMutation 响应修改结果
type ResponseMutation struct {
	StatusCode    *int
	Headers       map[string]string
	RemoveHeaders []string
	Body          *string
}

// Options 执行器配置
type Options struct {
	MaxCaptureSize int64         // 响应体采集限制
	ProcessTimeout time.Duration // 处理超时
}

// Executor 行为执行器（单次请求生命周期绑定）
type Executor struct {
	log    logger.Logger
	ev     *fetch.RequestPausedReply
	opts   Options
	reqMut *RequestMutation
	resMut *ResponseMutation
	block  *BlockResponse // 终结性行为状态
}

// New 创建行为执行器
func New(log logger.Logger, ev *fetch.RequestPausedReply, opts Options) *Executor {
	return &Executor{
		log:  log,
		ev:   ev,
		opts: opts,
	}
}

// ExecutionResult 汇总单次执行的结果
type ExecutionResult struct {
	IsBlocked    bool
	IsModified   bool
	IsLongConn   bool
	ContinueArgs *fetch.ContinueRequestArgs
	FulfillArgs  *fetch.FulfillRequestArgs
	ContinueRes  *fetch.ContinueResponseArgs
}

// ExecuteRequest 批量执行请求阶段动作
func (e *Executor) ExecuteRequest(matchedRules []*rules.MatchedRule) *ExecutionResult {
	e.reqMut = &RequestMutation{
		Headers:       make(map[string]string),
		Query:         make(map[string]string),
		Cookies:       make(map[string]string),
		RemoveHeaders: []string{},
		RemoveQuery:   []string{},
		RemoveCookies: []string{},
	}

	for _, mr := range matchedRules {
		if mr.Rule.Stage != rulespec.StageRequest {
			continue
		}
		mut := e.ExecuteRequestActions(mr.Rule.Actions)
		if e.block != nil {
			break
		}
		e.reqMut.Merge(mut)
	}

	res := &ExecutionResult{
		IsBlocked:  e.block != nil,
		IsModified: e.HasRequestMutation(),
		IsLongConn: e.IsLongConnectionType(),
	}
	res.ContinueArgs, res.FulfillArgs = e.BuildRequestArgs(e.reqMut)
	return res
}

// Merge 合并请求修改结果
func (m *RequestMutation) Merge(src *RequestMutation) {
	if src == nil {
		return
	}
	if src.URL != nil {
		m.URL = src.URL
	}
	if src.Method != nil {
		m.Method = src.Method
	}
	for k, v := range src.Headers {
		if m.Headers == nil {
			m.Headers = make(map[string]string)
		}
		m.Headers[k] = v
	}
	for k, v := range src.Query {
		if m.Query == nil {
			m.Query = make(map[string]string)
		}
		m.Query[k] = v
	}
	for k, v := range src.Cookies {
		if m.Cookies == nil {
			m.Cookies = make(map[string]string)
		}
		m.Cookies[k] = v
	}
	m.RemoveHeaders = append(m.RemoveHeaders, src.RemoveHeaders...)
	m.RemoveQuery = append(m.RemoveQuery, src.RemoveQuery...)
	m.RemoveCookies = append(m.RemoveCookies, src.RemoveCookies...)
	if src.Body != nil {
		m.Body = src.Body
	}
}

// ExecuteResponse 批量执行响应阶段动作
func (e *Executor) ExecuteResponse(matchedRules []*rules.MatchedRule, originalBody string) *ExecutionResult {
	e.resMut = &ResponseMutation{
		Headers:       make(map[string]string),
		RemoveHeaders: []string{},
	}
	currentBody := originalBody

	for _, mr := range matchedRules {
		if mr.Rule.Stage != rulespec.StageResponse {
			continue
		}
		mut := e.ExecuteResponseActions(mr.Rule.Actions, currentBody)
		e.resMut.Merge(mut)
		if mut.Body != nil {
			currentBody = *mut.Body
		}
	}

	// 自动补充 Body 变更（如果合并后的 Body 发生了变化但没有显式设置）
	if e.resMut.Body == nil && currentBody != originalBody {
		e.resMut.Body = &currentBody
	}

	res := &ExecutionResult{
		IsModified: e.HasResponseMutation(),
	}
	res.ContinueRes, res.FulfillArgs = e.BuildResponseArgs(e.resMut)
	return res
}

// Merge 合并响应修改结果
func (m *ResponseMutation) Merge(src *ResponseMutation) {
	if src == nil {
		return
	}
	if src.StatusCode != nil {
		m.StatusCode = src.StatusCode
	}
	for k, v := range src.Headers {
		if m.Headers == nil {
			m.Headers = make(map[string]string)
		}
		m.Headers[k] = v
	}
	m.RemoveHeaders = append(m.RemoveHeaders, src.RemoveHeaders...)
	if src.Body != nil {
		m.Body = src.Body
	}
}

// HasRequestMutation 检查是否有实际的请求变更
func (e *Executor) HasRequestMutation() bool {
	if e.block != nil {
		return true
	}
	m := e.reqMut
	if m == nil {
		return false
	}
	return m.URL != nil || m.Method != nil ||
		len(m.Headers) > 0 || len(m.Query) > 0 || len(m.Cookies) > 0 ||
		len(m.RemoveHeaders) > 0 || len(m.RemoveQuery) > 0 || len(m.RemoveCookies) > 0 ||
		m.Body != nil
}

// HasResponseMutation 检查是否有实际的响应变更
func (e *Executor) HasResponseMutation() bool {
	m := e.resMut
	if m == nil {
		return false
	}
	return m.StatusCode != nil || len(m.Headers) > 0 || len(m.RemoveHeaders) > 0 || m.Body != nil
}

// Block 返回当前的阻止行为状态
func (e *Executor) Block() *BlockResponse {
	return e.block
}

// ExecuteRequestActions 执行单个规则内的动作序列
func (e *Executor) ExecuteRequestActions(actions []rulespec.Action) *RequestMutation {
	mut := &RequestMutation{
		Headers:       make(map[string]string),
		Query:         make(map[string]string),
		Cookies:       make(map[string]string),
		RemoveHeaders: []string{},
		RemoveQuery:   []string{},
		RemoveCookies: []string{},
	}

	// 获取当前请求体用于修改
	currentBody := protocol.GetRequestBody(e.ev)

	for _, action := range actions {
		switch action.Type {
		case rulespec.ActionSetUrl:
			if v, ok := action.Value.(string); ok {
				mut.URL = &v
			}

		case rulespec.ActionSetMethod:
			if v, ok := action.Value.(string); ok {
				mut.Method = &v
			}

		case rulespec.ActionSetHeader:
			if v, ok := action.Value.(string); ok {
				mut.Headers[action.Name] = v
			}

		case rulespec.ActionRemoveHeader:
			mut.RemoveHeaders = append(mut.RemoveHeaders, action.Name)

		case rulespec.ActionSetQueryParam:
			if v, ok := action.Value.(string); ok {
				mut.Query[action.Name] = v
			}

		case rulespec.ActionRemoveQueryParam:
			mut.RemoveQuery = append(mut.RemoveQuery, action.Name)

		case rulespec.ActionSetCookie:
			if v, ok := action.Value.(string); ok {
				mut.Cookies[action.Name] = v
			}

		case rulespec.ActionRemoveCookie:
			mut.RemoveCookies = append(mut.RemoveCookies, action.Name)

		case rulespec.ActionSetBody:
			if v, ok := action.Value.(string); ok {
				body := v
				if action.GetEncoding() == rulespec.BodyEncodingBase64 {
					if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
						body = string(decoded)
					}
				}
				currentBody = body
				mut.Body = &currentBody
			}

		case rulespec.ActionAppendBody:
			if v, ok := action.Value.(string); ok {
				appendText := v
				if action.GetEncoding() == rulespec.BodyEncodingBase64 {
					if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
						appendText = string(decoded)
					}
				}
				currentBody = currentBody + appendText
				mut.Body = &currentBody
			}

		case rulespec.ActionReplaceBodyText:
			if action.ReplaceAll {
				currentBody = strings.ReplaceAll(currentBody, action.Search, action.Replace)
			} else {
				currentBody = strings.Replace(currentBody, action.Search, action.Replace, 1)
			}
			mut.Body = &currentBody

		case rulespec.ActionPatchBodyJson:
			newBody, err := e.applyJSONPatches(currentBody, action.Patches)
			if err == nil {
				currentBody = newBody
				mut.Body = &currentBody
			}

		case rulespec.ActionSetFormField:
			if v, ok := action.Value.(string); ok {
				currentBody = e.setFormField(currentBody, action.Name, v)
				mut.Body = &currentBody
			}

		case rulespec.ActionRemoveFormField:
			currentBody = e.removeFormField(currentBody, action.Name)
			mut.Body = &currentBody

		case rulespec.ActionBlock:
			// 终结性行为
			e.block = &BlockResponse{
				StatusCode: action.StatusCode,
				Headers:    action.Headers,
			}
			if action.Body != "" {
				body := action.Body
				if action.GetBodyEncoding() == rulespec.BodyEncodingBase64 {
					if decoded, err := base64.StdEncoding.DecodeString(action.Body); err == nil {
						e.block.Body = decoded
					} else {
						e.block.Body = []byte(body)
					}
				} else {
					e.block.Body = []byte(body)
				}
			}
			// 终结性行为，立即返回
			return mut
		}
	}

	return mut
}

// ExecuteResponseActions 执行响应阶段的行为，返回修改结果
func (e *Executor) ExecuteResponseActions(actions []rulespec.Action, responseBody string) *ResponseMutation {
	mut := &ResponseMutation{
		Headers:       make(map[string]string),
		RemoveHeaders: []string{},
	}

	currentBody := responseBody

	for _, action := range actions {
		switch action.Type {
		case rulespec.ActionSetStatus:
			if v, ok := action.Value.(float64); ok {
				code := int(v)
				mut.StatusCode = &code
			} else if v, ok := action.Value.(int); ok {
				mut.StatusCode = &v
			}

		case rulespec.ActionSetHeader:
			if v, ok := action.Value.(string); ok {
				mut.Headers[action.Name] = v
			}

		case rulespec.ActionRemoveHeader:
			mut.RemoveHeaders = append(mut.RemoveHeaders, action.Name)

		case rulespec.ActionSetBody:
			if v, ok := action.Value.(string); ok {
				body := v
				if action.GetEncoding() == rulespec.BodyEncodingBase64 {
					if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
						body = string(decoded)
					}
				}
				currentBody = body
				mut.Body = &currentBody
			}

		case rulespec.ActionAppendBody:
			if v, ok := action.Value.(string); ok {
				appendText := v
				if action.GetEncoding() == rulespec.BodyEncodingBase64 {
					if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
						appendText = string(decoded)
					}
				}
				currentBody = currentBody + appendText
				mut.Body = &currentBody
			}

		case rulespec.ActionReplaceBodyText:
			if action.ReplaceAll {
				currentBody = strings.ReplaceAll(currentBody, action.Search, action.Replace)
			} else {
				currentBody = strings.Replace(currentBody, action.Search, action.Replace, 1)
			}
			mut.Body = &currentBody

		case rulespec.ActionPatchBodyJson:
			newBody, err := e.applyJSONPatches(currentBody, action.Patches)
			if err == nil {
				currentBody = newBody
				mut.Body = &currentBody
			}
		}
	}

	return mut
}

// BuildRequestArgs 构建请求阶段的 CDP 参数
func (e *Executor) BuildRequestArgs(mut *RequestMutation) (*fetch.ContinueRequestArgs, *fetch.FulfillRequestArgs) {
	// 处理终结性行为 block
	if e.block != nil {
		args := &fetch.FulfillRequestArgs{
			RequestID:    e.ev.RequestID,
			ResponseCode: e.block.StatusCode,
		}
		if len(e.block.Headers) > 0 {
			args.ResponseHeaders = toHeaderEntries(e.block.Headers)
		}
		if len(e.block.Body) > 0 {
			args.Body = e.block.Body
		}
		return nil, args
	}

	// 构建 ContinueRequest 参数
	args := &fetch.ContinueRequestArgs{RequestID: e.ev.RequestID}

	// URL 修改（包含 Query 修改）
	finalURL := e.buildFinalURL(e.ev.Request.URL, mut)
	if finalURL != nil {
		args.URL = finalURL
	}

	// Method 修改
	if mut.Method != nil {
		args.Method = mut.Method
	}

	// Headers 修改
	headers := e.buildFinalHeaders(mut)
	if len(headers) > 0 {
		args.Headers = headers
	}

	// Body 修改
	if mut.Body != nil {
		args.PostData = []byte(*mut.Body)
	}

	return args, nil
}

// BuildResponseArgs 构建响应阶段的 CDP 参数
func (e *Executor) BuildResponseArgs(mut *ResponseMutation) (*fetch.ContinueResponseArgs, *fetch.FulfillRequestArgs) {
	// 如果需要修改 Body，必须使用 FulfillRequest
	if mut.Body != nil {
		code := 200
		if e.ev.ResponseStatusCode != nil {
			code = *e.ev.ResponseStatusCode
		}
		if mut.StatusCode != nil {
			code = *mut.StatusCode
		}

		headers := e.buildFinalResponseHeaders(mut)

		args := &fetch.FulfillRequestArgs{
			RequestID:       e.ev.RequestID,
			ResponseCode:    code,
			ResponseHeaders: headers,
			Body:            []byte(*mut.Body),
		}
		return nil, args
	}

	// 只修改状态码和头部，使用 ContinueResponse
	args := &fetch.ContinueResponseArgs{RequestID: e.ev.RequestID}

	// CDP 要求：如果要覆盖响应状态码或响应头，则两者必须同时提供，否则会报错
	hasMutation := mut.StatusCode != nil || len(mut.Headers) > 0 || len(mut.RemoveHeaders) > 0

	if hasMutation {
		code := 200
		if e.ev.ResponseStatusCode != nil {
			code = *e.ev.ResponseStatusCode
		}
		if mut.StatusCode != nil {
			code = *mut.StatusCode
		}
		args.ResponseCode = &code
		args.ResponseHeaders = e.buildFinalResponseHeaders(mut)
	}

	return args, nil
}

// FetchResponseBody 获取响应体
func (e *Executor) FetchResponseBody(ctx context.Context, client *cdp.Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("cdp client is nil")
	}

	timeout := e.opts.ProcessTimeout
	if timeout <= 0 {
		timeout = 500 * time.Millisecond // 默认保底超时
	}

	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	rb, err := client.Fetch.GetResponseBody(ctx2, &fetch.GetResponseBodyArgs{RequestID: e.ev.RequestID})
	if err != nil {
		return "", err
	}
	if rb == nil {
		return "", fmt.Errorf("response body is nil")
	}
	if rb.Base64Encoded {
		b, err := base64.StdEncoding.DecodeString(rb.Body)
		if err != nil {
			return "", fmt.Errorf("base64 decode failed: %w", err)
		}
		return string(b), nil
	}
	return rb.Body, nil
}

// buildFinalURL 构建最终 URL
func (e *Executor) buildFinalURL(originalURL string, mut *RequestMutation) *string {
	if mut.URL == nil && len(mut.Query) == 0 && len(mut.RemoveQuery) == 0 {
		return nil
	}

	baseURL := originalURL
	if mut.URL != nil {
		baseURL = *mut.URL
	}

	// 如果没有 Query 修改，直接返回
	if len(mut.Query) == 0 && len(mut.RemoveQuery) == 0 {
		return &baseURL
	}

	// 解析并修改 Query
	u, err := url.Parse(baseURL)
	if err != nil {
		return &baseURL
	}

	q := u.Query()
	// 移除参数
	for _, name := range mut.RemoveQuery {
		q.Del(name)
	}
	// 设置参数
	for name, value := range mut.Query {
		q.Set(name, value)
	}
	u.RawQuery = q.Encode()

	result := u.String()
	return &result
}

// buildFinalHeaders 构建最终请求头
func (e *Executor) buildFinalHeaders(mut *RequestMutation) []fetch.HeaderEntry {
	// 解析原始头部
	originalHeaders := make(map[string]string)
	if err := json.Unmarshal(e.ev.Request.Headers, &originalHeaders); err != nil {
		e.log.Err(err, "解析原始请求头失败", "requestID", e.ev.RequestID)
	}

	// 移除头部
	for _, name := range mut.RemoveHeaders {
		delete(originalHeaders, name)
		for k := range originalHeaders {
			if strings.EqualFold(k, name) {
				delete(originalHeaders, k)
			}
		}
	}

	// 设置头部
	for name, value := range mut.Headers {
		originalHeaders[name] = value
	}

	// 处理 Cookie 修改
	if len(mut.Cookies) > 0 || len(mut.RemoveCookies) > 0 {
		cookieStr := ""
		for k, v := range originalHeaders {
			if strings.EqualFold(k, "cookie") {
				cookieStr = v
				break
			}
		}
		cookies := protocol.ParseCookie(cookieStr)

		// 移除 Cookie
		for _, name := range mut.RemoveCookies {
			delete(cookies, name)
		}
		// 设置 Cookie
		for name, value := range mut.Cookies {
			cookies[name] = value
		}

		// 重新构建 Cookie 字符串
		if len(cookies) > 0 {
			var parts []string
			for k, v := range cookies {
				parts = append(parts, k+"="+v)
			}
			originalHeaders["Cookie"] = strings.Join(parts, "; ")
		} else {
			delete(originalHeaders, "Cookie")
			delete(originalHeaders, "cookie")
		}
	}

	return toHeaderEntries(originalHeaders)
}

// buildFinalResponseHeaders 构建最终响应头
func (e *Executor) buildFinalResponseHeaders(mut *ResponseMutation) []fetch.HeaderEntry {
	// 获取原始响应头
	headers := make(map[string]string)
	for _, h := range e.ev.ResponseHeaders {
		headers[h.Name] = h.Value
	}

	// 移除头部
	for _, name := range mut.RemoveHeaders {
		delete(headers, name)
		for k := range headers {
			if strings.EqualFold(k, name) {
				delete(headers, k)
			}
		}
	}

	// 如果 Body 被修改了，必须清理可能导致浏览器解析错误的特定头部
	if mut.Body != nil {
		headersToDrop := []string{"content-encoding", "content-length", "content-md5", "etag"}
		for _, name := range headersToDrop {
			delete(headers, name)
			for k := range headers {
				if strings.EqualFold(k, name) {
					delete(headers, k)
				}
			}
		}
	}

	// 设置头部
	for name, value := range mut.Headers {
		headers[name] = value
	}

	return toHeaderEntries(headers)
}

// applyJSONPatches 应用 JSON Patch 操作，使用 sjson 实现高性能修改
func (e *Executor) applyJSONPatches(body string, patches []rulespec.JSONPatchOp) (string, error) {
	if body == "" || len(patches) == 0 {
		return body, nil
	}

	currentBody := body

	for _, patch := range patches {
		if patch.Path == "" {
			continue
		}

		// 将 JSON Patch 路径 (/a/b/c) 转换为 sjson 路径 (a.b.c)
		path := patch.Path
		path = strings.TrimPrefix(path, "/")
		path = strings.ReplaceAll(path, "/", ".")

		var err error
		switch patch.Op {
		case "add", "replace":
			currentBody, err = sjson.Set(currentBody, path, patch.Value)
			if err != nil {
				e.log.Err(err, "sjson set error", "requestID", e.ev.RequestID, "path", path, "op", patch.Op)
				return body, err
			}
		case "remove":
			currentBody, err = sjson.Delete(currentBody, path)
			if err != nil {
				e.log.Err(err, "sjson delete error", "requestID", e.ev.RequestID, "path", path)
				return body, err
			}
		}
	}

	return currentBody, nil
}

// setFormField 设置表单字段
func (e *Executor) setFormField(body, name, value string) string {
	contentType := e.getContentType()

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return setURLEncodedField(body, name, value)
	}

	if strings.Contains(contentType, "multipart/form-data") {
		// TODO: 实现 multipart 表单修改
		return body
	}

	return body
}

// removeFormField 移除表单字段
func (e *Executor) removeFormField(body, name string) string {
	contentType := e.getContentType()

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return removeURLEncodedField(body, name)
	}

	if strings.Contains(contentType, "multipart/form-data") {
		// TODO: 实现 multipart 表单修改
		return body
	}

	return body
}

// getContentType 获取 Content-Type
func (e *Executor) getContentType() string {
	var headers map[string]string
	_ = json.Unmarshal(e.ev.Request.Headers, &headers)
	for k, v := range headers {
		if strings.EqualFold(k, "content-type") {
			return v
		}
	}
	return ""
}

// CaptureRequestSnapshot 捕获当前请求的最终快照（包含修改后的结果）
func (e *Executor) CaptureRequestSnapshot() domain.RequestInfo {
	// 获取原始信息
	req := domain.RequestInfo{
		URL:          e.ev.Request.URL,
		Method:       e.ev.Request.Method,
		Headers:      make(map[string]string),
		ResourceType: string(e.ev.ResourceType),
		Body:         protocol.GetRequestBody(e.ev),
	}
	_ = json.Unmarshal(e.ev.Request.Headers, &req.Headers)

	// 应用 Mutation 效果到审计快照
	if e.reqMut != nil {
		if e.reqMut.URL != nil {
			req.URL = *e.reqMut.URL
		}
		if e.reqMut.Method != nil {
			req.Method = *e.reqMut.Method
		}
		for _, name := range e.reqMut.RemoveHeaders {
			delete(req.Headers, name)
		}
		for name, val := range e.reqMut.Headers {
			req.Headers[name] = val
		}
		if e.reqMut.Body != nil {
			req.Body = *e.reqMut.Body
		}
	}
	return req
}

// CaptureResponseSnapshot 捕获当前响应的最终快照
func (e *Executor) CaptureResponseSnapshot(finalBody string) domain.ResponseInfo {
	res := domain.ResponseInfo{
		Headers: make(map[string]string),
		Body:    finalBody,
	}
	if e.ev.ResponseStatusCode != nil {
		res.StatusCode = *e.ev.ResponseStatusCode
	}
	for _, h := range e.ev.ResponseHeaders {
		res.Headers[h.Name] = h.Value
	}

	if e.resMut != nil {
		if e.resMut.StatusCode != nil {
			res.StatusCode = *e.resMut.StatusCode
		}
		for _, name := range e.resMut.RemoveHeaders {
			delete(res.Headers, name)
		}
		for name, val := range e.resMut.Headers {
			res.Headers[name] = val
		}
	}
	return res
}

// IsLongConnectionType 识别天生就是长连接的请求类型
func (e *Executor) IsLongConnectionType() bool {
	rt := string(e.ev.ResourceType)
	if rt == "WebSocket" || rt == "EventSource" {
		return true
	}
	// 解析 Header 检查 Upgrade
	var headers map[string]string
	_ = json.Unmarshal(e.ev.Request.Headers, &headers)
	for k, v := range headers {
		if strings.EqualFold(k, "upgrade") && strings.EqualFold(v, "websocket") {
			return true
		}
	}
	return false
}

// IsUnsafeResponseBody 识别不宜读取 Body 的响应（如大文件或流）
func (e *Executor) IsUnsafeResponseBody() (bool, string) {
	for _, h := range e.ev.ResponseHeaders {
		name := strings.ToLower(h.Name)
		if name == "content-length" {
			var size int64
			fmt.Sscanf(h.Value, "%d", &size)
			if size > e.opts.MaxCaptureSize && e.opts.MaxCaptureSize > 0 {
				return true, fmt.Sprintf("size exceeds limit (%d bytes)", size)
			}
		}
		if name == "content-type" {
			ct := strings.ToLower(h.Value)
			if strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/") ||
				strings.HasPrefix(ct, "text/event-stream") || ct == "application/octet-stream" {
				return true, "streaming or binary content-type: " + ct
			}
		}
	}
	return false, ""
}

// ToEvalContext 将 CDP 事件转换为规则引擎评估上下文
func ToEvalContext(ev *fetch.RequestPausedReply) *rules.EvalContext {
	headers := map[string]string{}
	query := map[string]string{}
	cookies := map[string]string{}
	var resourceType string

	if ev.ResourceType != "" {
		resourceType = string(ev.ResourceType)
	}

	_ = json.Unmarshal(ev.Request.Headers, &headers)
	if len(headers) > 0 {
		normalized := make(map[string]string, len(headers))
		for k, v := range headers {
			normalized[strings.ToLower(k)] = v
		}
		headers = normalized
	}

	if ev.Request.URL != "" {
		if u, err := url.Parse(ev.Request.URL); err == nil {
			for key, vals := range u.Query() {
				if len(vals) > 0 {
					query[strings.ToLower(key)] = vals[0]
				}
			}
		}
	}

	if v, ok := headers["cookie"]; ok {
		for name, val := range protocol.ParseCookie(v) {
			cookies[strings.ToLower(name)] = val
		}
	}

	return &rules.EvalContext{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		ResourceType: resourceType,
		Headers:      headers,
		Query:        query,
		Cookies:      cookies,
		Body:         protocol.GetRequestBody(ev),
	}
}

// toHeaderEntries 将头部映射转换为 CDP 头部条目
func toHeaderEntries(h map[string]string) []fetch.HeaderEntry {
	out := make([]fetch.HeaderEntry, 0, len(h))
	for k, v := range h {
		out = append(out, fetch.HeaderEntry{Name: k, Value: v})
	}
	return out
}

// setURLEncodedField 设置 URL 编码表单字段
func setURLEncodedField(body, name, value string) string {
	values, err := url.ParseQuery(body)
	if err != nil {
		return body
	}
	values.Set(name, value)
	return values.Encode()
}

// removeURLEncodedField 移除 URL 编码表单字段
func removeURLEncodedField(body, name string) string {
	values, err := url.ParseQuery(body)
	if err != nil {
		return body
	}
	values.Del(name)
	return values.Encode()
}
