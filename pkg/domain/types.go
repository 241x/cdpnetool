package domain

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

// RequestInfo 请求信息
type RequestInfo struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers"`
	Body         string            `json:"body"`
	ResourceType string            `json:"resourceType,omitempty"`
}

// ResponseInfo 响应信息
type ResponseInfo struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Timing     ResponseTiming    `json:"timing,omitempty"`
}

// ResponseTiming 响应时间信息
type ResponseTiming struct {
	StartTime int64 `json:"startTime"`
	EndTime   int64 `json:"endTime"`
}

// RuleMatch 规则匹配信息
type RuleMatch struct {
	RuleID   string   `json:"ruleId"`
	RuleName string   `json:"ruleName"`
	Actions  []string `json:"actions"`
}

// NetworkEvent 网络请求事件（统一所有拦截事件）
type NetworkEvent struct {
	Session      SessionID    `json:"session"`
	Target       TargetID     `json:"target"`
	Timestamp    int64        `json:"timestamp"`
	IsMatched    bool         `json:"isMatched"` // 是否匹配规则
	Request      RequestInfo  `json:"request"`
	Response     ResponseInfo `json:"response,omitempty"`
	FinalResult  string       `json:"finalResult,omitempty"`  // blocked / modified / passed
	MatchedRules []RuleMatch  `json:"matchedRules,omitempty"` // 匹配的规则列表
}
