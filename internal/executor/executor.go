package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/tidwall/sjson"

	"cdpnetool/internal/protocol"
	"cdpnetool/pkg/rulespec"
)

// Executor 行为执行器
type Executor struct{}

// New 创建行为执行器
func New() *Executor {
	return &Executor{}
}

// ExecuteRequestActions 执行请求阶段的行为，返回修改结果
func (e *Executor) ExecuteRequestActions(actions []rulespec.Action, ev *fetch.RequestPausedReply) *RequestMutation {
	mut := &RequestMutation{
		Headers:       make(map[string]string),
		Query:         make(map[string]string),
		Cookies:       make(map[string]string),
		RemoveHeaders: []string{},
		RemoveQuery:   []string{},
		RemoveCookies: []string{},
	}

	// 获取当前请求体用于修改
	currentBody := protocol.GetRequestBody(ev)

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

		case rulespec.ActionReplaceBodyText:
			if action.ReplaceAll {
				currentBody = strings.ReplaceAll(currentBody, action.Search, action.Replace)
			} else {
				currentBody = strings.Replace(currentBody, action.Search, action.Replace, 1)
			}
			mut.Body = &currentBody

		case rulespec.ActionPatchBodyJson:
			if newBody, ok := applyJSONPatches(currentBody, action.Patches); ok {
				currentBody = newBody
				mut.Body = &currentBody
			}

		case rulespec.ActionSetFormField:
			if v, ok := action.Value.(string); ok {
				currentBody = setFormField(currentBody, action.Name, v, ev)
				mut.Body = &currentBody
			}

		case rulespec.ActionRemoveFormField:
			currentBody = removeFormField(currentBody, action.Name, ev)
			mut.Body = &currentBody

		case rulespec.ActionBlock:
			// 终结性行为
			mut.Block = &BlockResponse{
				StatusCode: action.StatusCode,
				Headers:    action.Headers,
			}
			if action.Body != "" {
				body := action.Body
				if action.GetBodyEncoding() == rulespec.BodyEncodingBase64 {
					if decoded, err := base64.StdEncoding.DecodeString(action.Body); err == nil {
						mut.Block.Body = decoded
					} else {
						mut.Block.Body = []byte(body)
					}
				} else {
					mut.Block.Body = []byte(body)
				}
			}
			return mut // 终结性行为，立即返回
		}
	}

	return mut
}

// ExecuteResponseActions 执行响应阶段的行为，返回修改结果
func (e *Executor) ExecuteResponseActions(actions []rulespec.Action, ev *fetch.RequestPausedReply, responseBody string) *ResponseMutation {
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

		case rulespec.ActionReplaceBodyText:
			if action.ReplaceAll {
				currentBody = strings.ReplaceAll(currentBody, action.Search, action.Replace)
			} else {
				currentBody = strings.Replace(currentBody, action.Search, action.Replace, 1)
			}
			mut.Body = &currentBody

		case rulespec.ActionPatchBodyJson:
			if newBody, ok := applyJSONPatches(currentBody, action.Patches); ok {
				currentBody = newBody
				mut.Body = &currentBody
			}
		}
	}

	return mut
}

// ApplyRequestMutation 应用请求修改到 CDP
func (e *Executor) ApplyRequestMutation(ctx context.Context, client *cdp.Client, ev *fetch.RequestPausedReply, mut *RequestMutation) {
	if client == nil {
		return
	}

	// 处理终结性行为 block
	if mut.Block != nil {
		args := &fetch.FulfillRequestArgs{
			RequestID:    ev.RequestID,
			ResponseCode: mut.Block.StatusCode,
		}
		if len(mut.Block.Headers) > 0 {
			args.ResponseHeaders = toHeaderEntries(mut.Block.Headers)
		}
		if len(mut.Block.Body) > 0 {
			args.Body = mut.Block.Body
		}
		_ = client.Fetch.FulfillRequest(ctx, args)
		return
	}

	// 构建 ContinueRequest 参数
	args := &fetch.ContinueRequestArgs{RequestID: ev.RequestID}

	// URL 修改（包含 Query 修改）
	finalURL := e.buildFinalURL(ev.Request.URL, mut)
	if finalURL != nil {
		args.URL = finalURL
	}

	// Method 修改
	if mut.Method != nil {
		args.Method = mut.Method
	}

	// Headers 修改
	headers := e.buildFinalHeaders(ev, mut)
	if len(headers) > 0 {
		args.Headers = headers
	}

	// Body 修改
	if mut.Body != nil {
		args.PostData = []byte(*mut.Body)
	}

	_ = client.Fetch.ContinueRequest(ctx, args)
}

// ApplyResponseMutation 应用响应修改到 CDP
func (e *Executor) ApplyResponseMutation(ctx context.Context, client *cdp.Client, ev *fetch.RequestPausedReply, mut *ResponseMutation) {
	if client == nil {
		return
	}

	// 如果需要修改 Body，必须使用 FulfillRequest
	if mut.Body != nil {
		code := 200
		if ev.ResponseStatusCode != nil {
			code = *ev.ResponseStatusCode
		}
		if mut.StatusCode != nil {
			code = *mut.StatusCode
		}

		headers := e.buildFinalResponseHeaders(ev, mut)

		args := &fetch.FulfillRequestArgs{
			RequestID:       ev.RequestID,
			ResponseCode:    code,
			ResponseHeaders: headers,
			Body:            []byte(*mut.Body),
		}
		_ = client.Fetch.FulfillRequest(ctx, args)
		return
	}

	// 只修改状态码和头部，使用 ContinueResponse
	args := &fetch.ContinueResponseArgs{RequestID: ev.RequestID}
	if mut.StatusCode != nil {
		args.ResponseCode = mut.StatusCode
	}

	headers := e.buildFinalResponseHeaders(ev, mut)
	if len(headers) > 0 {
		args.ResponseHeaders = headers
	}
	_ = client.Fetch.ContinueResponse(ctx, args)
}

// ContinueRequest 继续原请求
func (e *Executor) ContinueRequest(ctx context.Context, client *cdp.Client, ev *fetch.RequestPausedReply) {
	if client == nil {
		return
	}
	_ = client.Fetch.ContinueRequest(ctx, &fetch.ContinueRequestArgs{RequestID: ev.RequestID})
}

// ContinueResponse 继续原响应
func (e *Executor) ContinueResponse(ctx context.Context, client *cdp.Client, ev *fetch.RequestPausedReply) {
	if client == nil {
		return
	}
	_ = client.Fetch.ContinueResponse(ctx, &fetch.ContinueResponseArgs{RequestID: ev.RequestID})
}

// FetchResponseBody 获取响应体
func (e *Executor) FetchResponseBody(ctx context.Context, client *cdp.Client, requestID fetch.RequestID) (string, bool) {
	if client == nil {
		return "", false
	}
	ctx2, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	rb, err := client.Fetch.GetResponseBody(ctx2, &fetch.GetResponseBodyArgs{RequestID: requestID})
	if err != nil || rb == nil {
		return "", false
	}
	if rb.Base64Encoded {
		if b, err := base64.StdEncoding.DecodeString(rb.Body); err == nil {
			return string(b), true
		}
		return "", false
	}
	return rb.Body, true
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
func (e *Executor) buildFinalHeaders(ev *fetch.RequestPausedReply, mut *RequestMutation) []fetch.HeaderEntry {
	// 解析原始头部
	originalHeaders := make(map[string]string)
	_ = json.Unmarshal(ev.Request.Headers, &originalHeaders)

	// 应用修改
	// 1. 移除头部
	for _, name := range mut.RemoveHeaders {
		delete(originalHeaders, name)
		// 不区分大小写删除
		for k := range originalHeaders {
			if strings.EqualFold(k, name) {
				delete(originalHeaders, k)
			}
		}
	}

	// 2. 设置头部
	for name, value := range mut.Headers {
		originalHeaders[name] = value
	}

	// 3. 处理 Cookie 修改
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
func (e *Executor) buildFinalResponseHeaders(ev *fetch.RequestPausedReply, mut *ResponseMutation) []fetch.HeaderEntry {
	// 获取原始响应头
	headers := make(map[string]string)
	for _, h := range ev.ResponseHeaders {
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

	// 设置头部
	for name, value := range mut.Headers {
		headers[name] = value
	}

	return toHeaderEntries(headers)
}

// toHeaderEntries 将头部映射转换为 CDP 头部条目
func toHeaderEntries(h map[string]string) []fetch.HeaderEntry {
	out := make([]fetch.HeaderEntry, 0, len(h))
	for k, v := range h {
		out = append(out, fetch.HeaderEntry{Name: k, Value: v})
	}
	return out
}

// applyJSONPatches 应用 JSON Patch 操作，使用 sjson 实现高性能修改
func applyJSONPatches(body string, patches []rulespec.JSONPatchOp) (string, bool) {
	if body == "" || len(patches) == 0 {
		return body, false
	}

	currentBody := body
	modified := false

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
			if err == nil {
				modified = true
			}
		case "remove":
			currentBody, err = sjson.Delete(currentBody, path)
			if err == nil {
				modified = true
			}
		}
	}

	return currentBody, modified
}

// setFormField 设置表单字段
func setFormField(body, name, value string, ev *fetch.RequestPausedReply) string {
	contentType := getContentType(ev)

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
func removeFormField(body, name string, ev *fetch.RequestPausedReply) string {
	contentType := getContentType(ev)

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return removeURLEncodedField(body, name)
	}

	if strings.Contains(contentType, "multipart/form-data") {
		// TODO: 实现 multipart 表单修改
		return body
	}

	return body
}

// setURLEncodedField 设置 URL 编码表单字段
func setURLEncodedField(body, name, value string) string {
	values, _ := url.ParseQuery(body)
	values.Set(name, value)
	return values.Encode()
}

// removeURLEncodedField 移除 URL 编码表单字段
func removeURLEncodedField(body, name string) string {
	values, _ := url.ParseQuery(body)
	values.Del(name)
	return values.Encode()
}

// getContentType 获取 Content-Type
func getContentType(ev *fetch.RequestPausedReply) string {
	var headers map[string]string
	_ = json.Unmarshal(ev.Request.Headers, &headers)

	for k, v := range headers {
		if strings.EqualFold(k, "content-type") {
			return v
		}
	}
	return ""
}
