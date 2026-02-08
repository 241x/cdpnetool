package cdp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"

	"cdpnetool/internal/transformer"
	"cdpnetool/pkg/domain"

	"github.com/mafredri/cdp/protocol/fetch"
)

// ToNeutralRequest 将 CDP 事件转换为领域 Request 模型
func ToNeutralRequest(ev *fetch.RequestPausedReply) *domain.Request {
	req := domain.NewRequest()
	req.ID = string(ev.RequestID)
	req.URL = ev.Request.URL
	req.Method = ev.Request.Method

	// 使用智能归类函数将 CDP 的 ResourceType 转换为我们的规范类型
	req.ResourceType = domain.NormalizeResourceType(string(ev.ResourceType), ev.Request.URL)

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
					req.Query[kv[0]] = kv[1]
				}
			}
		}
	}

	// 解析 Cookie
	req.Cookies = transformer.ParseCookies(req.Headers.Get("Cookie"))

	// 处理请求体：优先使用 PostDataEntries（支持大数据），回退到 PostData（已废弃）
	if len(ev.Request.PostDataEntries) > 0 {
		// PostDataEntries.Bytes 是 Base64 编码，需要解码
		var bodyParts [][]byte
		for _, entry := range ev.Request.PostDataEntries {
			if entry.Bytes != nil {
				decodedBytes, err := base64.StdEncoding.DecodeString(*entry.Bytes)
				if err != nil {
					bodyParts = append(bodyParts, []byte(*entry.Bytes))
				} else {
					bodyParts = append(bodyParts, decodedBytes)
				}
			}
		}
		if len(bodyParts) > 0 {
			req.Body = bytes.Join(bodyParts, nil)
		}
	} else if ev.Request.PostData != nil {
		// PostData 是原始字符串，直接使用
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
