package cdp

import (
	"encoding/json"
	"strings"

	"cdpnetool/pkg/domain"

	"github.com/mafredri/cdp/protocol/fetch"
)

// ToNeutralRequest 将 CDP 事件转换为领域 Request 模型
func ToNeutralRequest(ev *fetch.RequestPausedReply) *domain.Request {
	req := domain.NewRequest()
	req.ID = string(ev.RequestID)
	req.URL = ev.Request.URL
	req.Method = ev.Request.Method
	req.ResourceType = string(ev.ResourceType)

	// 处理 Header
	var headers map[string]string
	if len(ev.Request.Headers) > 0 {
		if err := json.Unmarshal(ev.Request.Headers, &headers); err == nil {
			for k, v := range headers {
				req.Headers.Set(k, v)
			}
		}
	}

	// 解析 Query 参数
	if idx := strings.Index(req.URL, "?"); idx != -1 {
		queryStr := req.URL[idx+1:]
		if queryStr != "" {
			for _, pair := range strings.Split(queryStr, "&") {
				if kv := strings.SplitN(pair, "=", 2); len(kv) == 2 {
					req.Query[strings.ToLower(kv[0])] = kv[1]
				}
			}
		}
	}

	// 解析 Cookie
	if cookieHeader := req.Headers.Get("cookie"); cookieHeader != "" {
		for _, pair := range strings.Split(cookieHeader, ";") {
			pair = strings.TrimSpace(pair)
			if kv := strings.SplitN(pair, "=", 2); len(kv) == 2 {
				req.Cookies[strings.ToLower(kv[0])] = kv[1]
			}
		}
	}

	// 处理请求体 (PostData)
	if ev.Request.PostData != nil {
		req.Body = []byte(*ev.Request.PostData)
	}

	return req
}

// ToNeutralResponse 将 CDP 事件转换为领域 Response 模型
func ToNeutralResponse(ev *fetch.RequestPausedReply, body []byte) *domain.Response {
	res := domain.NewResponse()
	if ev.ResponseStatusCode != nil {
		res.StatusCode = *ev.ResponseStatusCode
	}
	for _, h := range ev.ResponseHeaders {
		res.Headers.Set(h.Name, h.Value)
	}
	res.Body = body
	return res
}

// ToHeaderEntries 将领域 Header 转换为 CDP Header 条目
func ToHeaderEntries(h domain.Header) []fetch.HeaderEntry {
	entries := make([]fetch.HeaderEntry, 0, len(h))
	for k, v := range h {
		entries = append(entries, fetch.HeaderEntry{Name: k, Value: v})
	}
	return entries
}
