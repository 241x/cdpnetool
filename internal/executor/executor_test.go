package executor_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"cdpnetool/internal/executor"
	"cdpnetool/internal/logger"
	"cdpnetool/pkg/rulespec"

	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/network"
)

// TestExecutor_ExecuteRequestActions 表驱动测试请求阶段的行为执行
func TestExecutor_ExecuteRequestActions(t *testing.T) {
	tests := []struct {
		name     string
		actions  []rulespec.Action
		ev       *fetch.RequestPausedReply
		validate func(*testing.T, *executor.Executor, *executor.RequestMutation)
	}{
		{
			name: "设置 URL",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetUrl, Value: "https://new-url.com"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.URL == nil {
					t.Fatal("expected URL to be set")
				}
				if *mut.URL != "https://new-url.com" {
					t.Errorf("expected URL 'https://new-url.com', got '%s'", *mut.URL)
				}
			},
		},
		{
			name: "设置请求方法",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetMethod, Value: "POST"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.Method == nil {
					t.Fatal("expected Method to be set")
				}
				if *mut.Method != "POST" {
					t.Errorf("expected Method 'POST', got '%s'", *mut.Method)
				}
			},
		},
		{
			name: "设置多个请求头",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetHeader, Name: "X-Custom-A", Value: "value-a"},
				{Type: rulespec.ActionSetHeader, Name: "X-Custom-B", Value: "value-b"},
				{Type: rulespec.ActionSetHeader, Name: "Authorization", Value: "Bearer token"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if len(mut.Headers) != 3 {
					t.Errorf("expected 3 headers, got %d", len(mut.Headers))
				}
				if mut.Headers["X-Custom-A"] != "value-a" {
					t.Errorf("expected X-Custom-A 'value-a', got '%s'", mut.Headers["X-Custom-A"])
				}
				if mut.Headers["Authorization"] != "Bearer token" {
					t.Errorf("expected Authorization header")
				}
			},
		},
		{
			name: "移除请求头",
			actions: []rulespec.Action{
				{Type: rulespec.ActionRemoveHeader, Name: "Authorization"},
				{Type: rulespec.ActionRemoveHeader, Name: "Cookie"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if len(mut.RemoveHeaders) != 2 {
					t.Errorf("expected 2 remove headers, got %d", len(mut.RemoveHeaders))
				}
				if mut.RemoveHeaders[0] != "Authorization" {
					t.Errorf("expected first remove header 'Authorization'")
				}
			},
		},
		{
			name: "设置查询参数",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetQueryParam, Name: "page", Value: "2"},
				{Type: rulespec.ActionSetQueryParam, Name: "limit", Value: "50"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if len(mut.Query) != 2 {
					t.Errorf("expected 2 query params, got %d", len(mut.Query))
				}
				if mut.Query["page"] != "2" {
					t.Errorf("expected page '2', got '%s'", mut.Query["page"])
				}
				if mut.Query["limit"] != "50" {
					t.Errorf("expected limit '50', got '%s'", mut.Query["limit"])
				}
			},
		},
		{
			name: "移除查询参数",
			actions: []rulespec.Action{
				{Type: rulespec.ActionRemoveQueryParam, Name: "utm_source"},
				{Type: rulespec.ActionRemoveQueryParam, Name: "utm_medium"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if len(mut.RemoveQuery) != 2 {
					t.Errorf("expected 2 remove query params, got %d", len(mut.RemoveQuery))
				}
			},
		},
		{
			name: "设置 Cookie",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetCookie, Name: "session_id", Value: "abc123"},
				{Type: rulespec.ActionSetCookie, Name: "user_id", Value: "999"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if len(mut.Cookies) != 2 {
					t.Errorf("expected 2 cookies, got %d", len(mut.Cookies))
				}
				if mut.Cookies["session_id"] != "abc123" {
					t.Errorf("expected session_id 'abc123'")
				}
			},
		},
		{
			name: "移除 Cookie",
			actions: []rulespec.Action{
				{Type: rulespec.ActionRemoveCookie, Name: "tracking_id"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if len(mut.RemoveCookies) != 1 {
					t.Errorf("expected 1 remove cookie, got %d", len(mut.RemoveCookies))
				}
				if mut.RemoveCookies[0] != "tracking_id" {
					t.Errorf("expected remove cookie 'tracking_id'")
				}
			},
		},
		{
			name: "设置请求体（纯文本）",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetBody, Value: "new body content"},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "new body content" {
					t.Errorf("expected body 'new body content', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "设置请求体（Base64）",
			actions: []rulespec.Action{
				{
					Type:     rulespec.ActionSetBody,
					Value:    base64.StdEncoding.EncodeToString([]byte("decoded content")),
					Encoding: rulespec.BodyEncodingBase64,
				},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "decoded content" {
					t.Errorf("expected decoded body 'decoded content', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "替换请求体文本（单次）",
			actions: []rulespec.Action{
				{
					Type:       rulespec.ActionReplaceBodyText,
					Search:     "old",
					Replace:    "new",
					ReplaceAll: false,
				},
			},
			ev: createRequestWithPostData("old old old"),
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "new old old" {
					t.Errorf("expected 'new old old', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "替换请求体文本（全部）",
			actions: []rulespec.Action{
				{
					Type:       rulespec.ActionReplaceBodyText,
					Search:     "old",
					Replace:    "new",
					ReplaceAll: true,
				},
			},
			ev: createRequestWithPostData("old old old"),
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "new new new" {
					t.Errorf("expected 'new new new', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "JSON Patch 操作",
			actions: []rulespec.Action{
				{
					Type: rulespec.ActionPatchBodyJson,
					Patches: []rulespec.JSONPatchOp{
						{Op: "replace", Path: "/name", Value: "Alice"},
						{Op: "add", Path: "/age", Value: float64(30)},
					},
				},
			},
			ev: createRequestWithPostData(`{"name":"Bob","email":"bob@example.com"}`),
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				var result map[string]interface{}
				if err := json.Unmarshal([]byte(*mut.Body), &result); err != nil {
					t.Fatalf("failed to parse JSON: %v", err)
				}
				if result["name"] != "Alice" {
					t.Errorf("expected name 'Alice', got '%v'", result["name"])
				}
				if result["age"] != float64(30) {
					t.Errorf("expected age 30, got '%v'", result["age"])
				}
			},
		},
		{
			name: "阻止请求（Block）",
			actions: []rulespec.Action{
				{
					Type:       rulespec.ActionBlock,
					StatusCode: 403,
					Headers:    map[string]string{"X-Blocked": "true"},
					Body:       "Access Denied",
				},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				block := exec.Block()
				if block == nil {
					t.Fatal("expected Block to be set")
				}
				if block.StatusCode != 403 {
					t.Errorf("expected status 403, got %d", block.StatusCode)
				}
				if block.Headers["X-Blocked"] != "true" {
					t.Errorf("expected X-Blocked header")
				}
				if string(block.Body) != "Access Denied" {
					t.Errorf("expected body 'Access Denied', got '%s'", string(block.Body))
				}
			},
		},
		{
			name: "阻止请求（带 Base64 Body）",
			actions: []rulespec.Action{
				{
					Type:         rulespec.ActionBlock,
					StatusCode:   404,
					Body:         base64.StdEncoding.EncodeToString([]byte("Not Found")),
					BodyEncoding: rulespec.BodyEncodingBase64,
				},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				block := exec.Block()
				if block == nil {
					t.Fatal("expected Block to be set")
				}
				if string(block.Body) != "Not Found" {
					t.Errorf("expected decoded body 'Not Found', got '%s'", string(block.Body))
				}
			},
		},
		{
			name: "组合操作（URL + Headers + Body）",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetUrl, Value: "https://api.example.com"},
				{Type: rulespec.ActionSetHeader, Name: "Content-Type", Value: "application/json"},
				{Type: rulespec.ActionSetBody, Value: `{"modified":true}`},
			},
			ev: &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut.URL == nil || *mut.URL != "https://api.example.com" {
					t.Error("URL not set correctly")
				}
				if mut.Headers["Content-Type"] != "application/json" {
					t.Error("Content-Type header not set")
				}
				if mut.Body == nil || *mut.Body != `{"modified":true}` {
					t.Error("Body not set correctly")
				}
			},
		},
		{
			name:    "空操作列表",
			actions: []rulespec.Action{},
			ev:      &fetch.RequestPausedReply{},
			validate: func(t *testing.T, exec *executor.Executor, mut *executor.RequestMutation) {
				if mut == nil {
					t.Fatal("expected non-nil mutation")
				}
				// 应该返回空的 mutation
				if mut.URL != nil || mut.Method != nil || mut.Body != nil {
					t.Error("expected empty mutation")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := executor.New(logger.NewNop(), tt.ev, executor.Options{})
			mut := exec.ExecuteRequestActions(tt.actions)
			if mut == nil {
				t.Fatal("expected non-nil mutation")
			}
			tt.validate(t, exec, mut)
		})
	}
}

// TestExecutor_ExecuteResponseActions 表驱动测试响应阶段的行为执行
func TestExecutor_ExecuteResponseActions(t *testing.T) {
	tests := []struct {
		name         string
		actions      []rulespec.Action
		ev           *fetch.RequestPausedReply
		responseBody string
		validate     func(*testing.T, *executor.ResponseMutation)
	}{
		{
			name: "设置响应状态码（float64）",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetStatus, Value: float64(201)},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.StatusCode == nil {
					t.Fatal("expected StatusCode to be set")
				}
				if *mut.StatusCode != 201 {
					t.Errorf("expected status 201, got %d", *mut.StatusCode)
				}
			},
		},
		{
			name: "设置响应状态码（int）",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetStatus, Value: 404},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.StatusCode == nil {
					t.Fatal("expected StatusCode to be set")
				}
				if *mut.StatusCode != 404 {
					t.Errorf("expected status 404, got %d", *mut.StatusCode)
				}
			},
		},
		{
			name: "设置响应头",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetHeader, Name: "X-Custom-Response", Value: "test-value"},
				{Type: rulespec.ActionSetHeader, Name: "Cache-Control", Value: "no-cache"},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if len(mut.Headers) != 2 {
					t.Errorf("expected 2 headers, got %d", len(mut.Headers))
				}
				if mut.Headers["X-Custom-Response"] != "test-value" {
					t.Error("X-Custom-Response header not set correctly")
				}
				if mut.Headers["Cache-Control"] != "no-cache" {
					t.Error("Cache-Control header not set correctly")
				}
			},
		},
		{
			name: "移除响应头",
			actions: []rulespec.Action{
				{Type: rulespec.ActionRemoveHeader, Name: "Set-Cookie"},
				{Type: rulespec.ActionRemoveHeader, Name: "X-Powered-By"},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if len(mut.RemoveHeaders) != 2 {
					t.Errorf("expected 2 remove headers, got %d", len(mut.RemoveHeaders))
				}
			},
		},
		{
			name: "设置响应体",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetBody, Value: "modified response body"},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "original response",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "modified response body" {
					t.Errorf("expected body 'modified response body', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "设置响应体（Base64）",
			actions: []rulespec.Action{
				{
					Type:     rulespec.ActionSetBody,
					Value:    base64.StdEncoding.EncodeToString([]byte("decoded response")),
					Encoding: rulespec.BodyEncodingBase64,
				},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "decoded response" {
					t.Errorf("expected 'decoded response', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "替换响应体文本（单次）",
			actions: []rulespec.Action{
				{
					Type:       rulespec.ActionReplaceBodyText,
					Search:     "error",
					Replace:    "success",
					ReplaceAll: false,
				},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "error: error occurred",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "success: error occurred" {
					t.Errorf("expected 'success: error occurred', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "替换响应体文本（全部）",
			actions: []rulespec.Action{
				{
					Type:       rulespec.ActionReplaceBodyText,
					Search:     "error",
					Replace:    "success",
					ReplaceAll: true,
				},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "error: error occurred",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				if *mut.Body != "success: success occurred" {
					t.Errorf("expected 'success: success occurred', got '%s'", *mut.Body)
				}
			},
		},
		{
			name: "JSON Patch 响应",
			actions: []rulespec.Action{
				{
					Type: rulespec.ActionPatchBodyJson,
					Patches: []rulespec.JSONPatchOp{
						{Op: "replace", Path: "/status", Value: "modified"},
						{Op: "remove", Path: "/debug"},
					},
				},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: `{"status":"original","debug":"info"}`,
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.Body == nil {
					t.Fatal("expected Body to be set")
				}
				var result map[string]interface{}
				if err := json.Unmarshal([]byte(*mut.Body), &result); err != nil {
					t.Fatalf("failed to parse JSON: %v", err)
				}
				if result["status"] != "modified" {
					t.Errorf("expected status 'modified', got '%v'", result["status"])
				}
				if _, exists := result["debug"]; exists {
					t.Error("expected debug field to be removed")
				}
			},
		},
		{
			name: "组合操作（状态码 + 头部 + Body）",
			actions: []rulespec.Action{
				{Type: rulespec.ActionSetStatus, Value: float64(200)},
				{Type: rulespec.ActionSetHeader, Name: "Content-Type", Value: "application/json"},
				{Type: rulespec.ActionSetBody, Value: `{"success":true}`},
			},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut.StatusCode == nil || *mut.StatusCode != 200 {
					t.Error("StatusCode not set correctly")
				}
				if mut.Headers["Content-Type"] != "application/json" {
					t.Error("Content-Type not set correctly")
				}
				if mut.Body == nil || *mut.Body != `{"success":true}` {
					t.Error("Body not set correctly")
				}
			},
		},
		{
			name:         "空操作列表",
			actions:      []rulespec.Action{},
			ev:           &fetch.RequestPausedReply{},
			responseBody: "original",
			validate: func(t *testing.T, mut *executor.ResponseMutation) {
				if mut == nil {
					t.Fatal("expected non-nil mutation")
				}
				if mut.StatusCode != nil || mut.Body != nil {
					t.Error("expected empty mutation")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := executor.New(logger.NewNop(), tt.ev, executor.Options{})
			mut := exec.ExecuteResponseActions(tt.actions, tt.responseBody)
			if mut == nil {
				t.Fatal("expected non-nil mutation")
			}
			tt.validate(t, mut)
		})
	}
}

// 辅助函数
func stringPtr(s string) *string {
	return &s
}

// createRequestWithPostData 创建带 PostData 的请求
func createRequestWithPostData(data string) *fetch.RequestPausedReply {
	return &fetch.RequestPausedReply{
		Request: network.Request{
			PostData: stringPtr(data),
		},
	}
}
