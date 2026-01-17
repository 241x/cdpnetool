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

// TargetInfo 目标信息
type TargetInfo struct {
	ID        TargetID `json:"id"`
	Type      string   `json:"type"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	IsCurrent bool     `json:"isCurrent"`
}

// ==================== 事件系统 ====================

// MatchedEvent 匹配的请求事件（会存入数据库）
type MatchedEvent struct {
	Session    SessionID `json:"session"`
	Target     TargetID  `json:"target"`
	URL        string    `json:"url"`
	Method     string    `json:"method"`
	Stage      string    `json:"stage"` // request / response
	StatusCode int       `json:"statusCode,omitempty"`
	Timestamp  int64     `json:"timestamp"`

	// 最终结果: blocked / modified / passed
	FinalResult string `json:"finalResult"`

	// 匹配的规则列表
	MatchedRules []RuleMatch `json:"matchedRules"`

	// 原始数据
	Original RequestResponseData `json:"original"`
	// 修改后的数据
	Modified RequestResponseData `json:"modified"`
}

// RuleMatch 规则匹配信息
type RuleMatch struct {
	RuleID   string   `json:"ruleId"`
	RuleName string   `json:"ruleName"`
	Actions  []string `json:"actions"` // 实际执行的 action 类型列表
}

// RequestResponseData 请求/响应数据
type RequestResponseData struct {
	URL          string            `json:"url,omitempty"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	PostData     string            `json:"postData,omitempty"` // POST 数据
	StatusCode   int               `json:"statusCode,omitempty"`
	ResourceType string            `json:"resourceType,omitempty"` // document/xhr/script/image等
}

// UnmatchedEvent 未匹配的请求事件（仅内存，不存数据库）
type UnmatchedEvent struct {
	Target     TargetID `json:"target"`
	URL        string   `json:"url"`
	Method     string   `json:"method"`
	Stage      string   `json:"stage"` // request / response
	StatusCode int      `json:"statusCode,omitempty"`
	Timestamp  int64    `json:"timestamp"`
}

// InterceptEvent 统一事件接口（用于通道传输）
type InterceptEvent struct {
	IsMatched bool `json:"isMatched"`

	// 匹配事件数据（IsMatched=true 时有效）
	Matched *MatchedEvent `json:"matched,omitempty"`

	// 未匹配事件数据（IsMatched=false 时有效）
	Unmatched *UnmatchedEvent `json:"unmatched,omitempty"`
}
