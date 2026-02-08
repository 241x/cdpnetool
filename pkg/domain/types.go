package domain

import (
	"net/http"
	"strings"
)

// SessionID 会话ID
type SessionID string

// TargetID 目标ID
type TargetID string

// RuleID 规则ID
type RuleID string

// ResourceType 资源类型
type ResourceType string

// ResourceType 枚举常量
const (
	ResourceTypeDocument   ResourceType = "document"   // HTML 文档
	ResourceTypeStylesheet ResourceType = "stylesheet" // CSS 样式表
	ResourceTypeImage      ResourceType = "image"      // 图片资源
	ResourceTypeMedia      ResourceType = "media"      // 音视频资源
	ResourceTypeFont       ResourceType = "font"       // 字体文件
	ResourceTypeScript     ResourceType = "script"     // JavaScript 脚本
	ResourceTypeXHR        ResourceType = "xhr"        // XMLHttpRequest
	ResourceTypeFetch      ResourceType = "fetch"      // Fetch API 请求
	ResourceTypeWebSocket  ResourceType = "websocket"  // WebSocket 连接
	ResourceTypeOther      ResourceType = "other"      // 其他未分类类型（包含所有特殊类型）
)

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

// Header 封装通用的头部操作
type Header map[string]string

// Get 获取指定 Header 的值
func (h Header) Get(key string) string {
	if h == nil {
		return ""
	}
	return h[key]
}

// Set 设置指定 Header 的值
func (h Header) Set(key, value string) {
	h[key] = value
}

// Del 删除指定 Header
func (h Header) Del(key string) {
	delete(h, key)
}

// Request 请求模型
type Request struct {
	ID           string            `json:"id"`                     // 事务唯一ID
	URL          string            `json:"url"`                    // 完整URL
	Method       string            `json:"method"`                 // HTTP方法
	Headers      Header            `json:"headers"`                // 请求头
	Body         []byte            `json:"body"`                   // 请求体原始数据
	ResourceType ResourceType      `json:"resourceType,omitempty"` // 资源类型
	Query        map[string]string `json:"query,omitempty"`        // 预解析的查询参数
	Cookies      map[string]string `json:"cookies,omitempty"`      // 预解析的Cookie
}

// Response 响应模型
type Response struct {
	StatusCode int            `json:"statusCode"`
	Headers    Header         `json:"headers"`
	Body       []byte         `json:"body"`
	Timing     ResponseTiming `json:"timing,omitempty"`
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
	ID           string      `json:"id"` // 事务唯一ID (CDP RequestID)
	Session      SessionID   `json:"session"`
	Target       TargetID    `json:"target"`
	Timestamp    int64       `json:"timestamp"`
	IsMatched    bool        `json:"isMatched"` // 是否匹配规则
	Request      Request     `json:"request"`
	Response     *Response   `json:"response,omitempty"`
	FinalResult  string      `json:"finalResult,omitempty"`  // blocked / modified / passed
	MatchedRules []RuleMatch `json:"matchedRules,omitempty"` // 匹配的规则列表
}

// NewRequest 创建初始化请求对象
func NewRequest() *Request {
	return &Request{
		Headers: make(Header),
		Query:   make(map[string]string),
		Cookies: make(map[string]string),
	}
}

// NewResponse 创建初始化响应对象
func NewResponse() *Response {
	return &Response{
		StatusCode: http.StatusOK,
		Headers:    make(Header),
	}
}

// NormalizeResourceType 将 CDP 原始 ResourceType 标准化为我们的规范类型
func NormalizeResourceType(cdpType string, url string) ResourceType {
	// 优先尝试从 URL 推断资源类型（适用于所有情况）
	if resType := guessTypeFromURL(url); resType != "" {
		return resType
	}

	// URL 无法判断，使用 CDP 原始类型
	cdpTypeLower := strings.ToLower(cdpType)

	// 映射标准类型
	switch ResourceType(cdpTypeLower) {
	case ResourceTypeDocument, ResourceTypeStylesheet, ResourceTypeImage,
		ResourceTypeMedia, ResourceTypeFont, ResourceTypeScript,
		ResourceTypeXHR, ResourceTypeFetch, ResourceTypeWebSocket:
		return ResourceType(cdpTypeLower)
	default:
		// 其他所有 CDP 类型归为 Other
		return ResourceTypeOther
	}
}

// guessTypeFromURL 根据 URL 扩展名推测资源类型
func guessTypeFromURL(url string) ResourceType {
	urlLower := strings.ToLower(url)

	// 移除查询参数和哈希
	if idx := strings.Index(urlLower, "?"); idx != -1 {
		urlLower = urlLower[:idx]
	}
	if idx := strings.Index(urlLower, "#"); idx != -1 {
		urlLower = urlLower[:idx]
	}

	// JavaScript 文件（只保留最常见的）
	if strings.HasSuffix(urlLower, ".js") || strings.HasSuffix(urlLower, ".mjs") {
		return ResourceTypeScript
	}

	// CSS 文件
	if strings.HasSuffix(urlLower, ".css") {
		return ResourceTypeStylesheet
	}

	// 图片文件（只保留最常见的）
	if strings.HasSuffix(urlLower, ".png") ||
		strings.HasSuffix(urlLower, ".jpg") ||
		strings.HasSuffix(urlLower, ".jpeg") ||
		strings.HasSuffix(urlLower, ".gif") ||
		strings.HasSuffix(urlLower, ".svg") ||
		strings.HasSuffix(urlLower, ".webp") {
		return ResourceTypeImage
	}

	// 字体文件
	if strings.HasSuffix(urlLower, ".woff") ||
		strings.HasSuffix(urlLower, ".woff2") ||
		strings.HasSuffix(urlLower, ".ttf") {
		return ResourceTypeFont
	}

	// 音视频文件
	if strings.HasSuffix(urlLower, ".mp4") ||
		strings.HasSuffix(urlLower, ".mp3") {
		return ResourceTypeMedia
	}

	// 无法推断，返回空（将由上层逻辑使用 CDP 类型）
	return ""
}
