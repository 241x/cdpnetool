package model

// SessionID 会话ID
type SessionID string

// TargetID 目标ID
type TargetID string

// RuleID 规则ID
type RuleID string

// SessionConfig 会话配置
type SessionConfig struct {
	DevToolsURL       string `json:"devToolsURL"`
	Concurrency       int    `json:"concurrency"`
	BodySizeThreshold int64  `json:"bodySizeThreshold"`
	PendingCapacity   int    `json:"pendingCapacity"`
	ProcessTimeoutMS  int    `json:"processTimeoutMS"`
}

// EngineStats 引擎统计信息
type EngineStats struct {
	Total   int64            `json:"total"`
	Matched int64            `json:"matched"`
	ByRule  map[RuleID]int64 `json:"byRule"`
}

// Event 事件
type Event struct {
	Type       string    `json:"type"`
	Session    SessionID `json:"session"`
	Target     TargetID  `json:"target"`
	Rule       *RuleID   `json:"rule,omitempty"`
	URL        string    `json:"url,omitempty"`
	Method     string    `json:"method,omitempty"`
	Stage      string    `json:"stage,omitempty"`
	StatusCode int       `json:"statusCode,omitempty"`
	Error      string    `json:"error,omitempty"`
	Timestamp  int64     `json:"timestamp"`
}

// TargetInfo 目标信息
type TargetInfo struct {
	ID        TargetID `json:"id"`
	Type      string   `json:"type"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	IsCurrent bool     `json:"isCurrent"`
}

// InterceptedRequest 拦截的请求
type InterceptedRequest struct {
	RequestID          string
	Stage              string
	URL                string
	Method             string
	RequestHeaders     map[string]string
	PostData           *string
	ResponseStatusCode *int
	ResponseHeaders    map[string]string
	ResponseBody       *string
}
