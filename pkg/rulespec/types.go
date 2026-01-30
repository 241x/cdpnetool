package rulespec

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"time"
)

// 配置版本常量
const (
	DefaultConfigVersion = "1.0"
)

// ID 格式约束
const (
	ConfigIDMinLen = 3
	ConfigIDMaxLen = 64
	RuleIDMinLen   = 1
	RuleIDMaxLen   = 64
)

// ID 格式正则
var (
	idPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// Config 配置文件根结构
type Config struct {
	ID          string         `json:"id"`          // 配置唯一标识符
	Name        string         `json:"name"`        // 配置名称
	Version     string         `json:"version"`     // 配置格式规范版本
	Description string         `json:"description"` // 配置描述
	Settings    map[string]any `json:"settings"`    // 预留设置项
	Rules       []Rule         `json:"rules"`       // 规则列表
}

// GenerateConfigID 生成配置 ID
func GenerateConfigID() string {
	dateStr := time.Now().Format("20060102")
	randomStr := generateRandomString(6)
	return fmt.Sprintf("config-%s-%s", dateStr, randomStr)
}

// GenerateRuleID 生成规则 ID
func GenerateRuleID(index int) string {
	return fmt.Sprintf("rule-%03d", index+1)
}

// ValidateConfigID 校验配置 ID 格式
func ValidateConfigID(id string) error {
	if len(id) < ConfigIDMinLen || len(id) > ConfigIDMaxLen {
		return fmt.Errorf("配置 ID 长度必须在 %d-%d 之间", ConfigIDMinLen, ConfigIDMaxLen)
	}
	if !idPattern.MatchString(id) {
		return fmt.Errorf("配置 ID 只能包含字母、数字、横线和下划线")
	}
	return nil
}

// ValidateRuleID 校验规则 ID 格式
func ValidateRuleID(id string) error {
	if len(id) < RuleIDMinLen || len(id) > RuleIDMaxLen {
		return fmt.Errorf("规则 ID 长度必须在 %d-%d 之间", RuleIDMinLen, RuleIDMaxLen)
	}
	if !idPattern.MatchString(id) {
		return fmt.Errorf("规则 ID 只能包含字母、数字、横线和下划线")
	}
	return nil
}

// generateRandomString 生成指定长度的随机字符串（字母+数字）
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

// NewConfig 创建一个新的空配置
func NewConfig(name string) *Config {
	return &Config{
		ID:          GenerateConfigID(),
		Name:        name,
		Version:     DefaultConfigVersion,
		Description: "",
		Settings:    map[string]any{},
		Rules:       []Rule{},
	}
}

// Stage 生命周期阶段
type Stage string

const (
	StageRequest  Stage = "request"  // 请求阶段
	StageResponse Stage = "response" // 响应阶段
)

// Rule 规则定义
type Rule struct {
	ID       string   `json:"id"`       // 规则唯一标识符
	Name     string   `json:"name"`     // 规则名称
	Enabled  bool     `json:"enabled"`  // 是否启用
	Priority int      `json:"priority"` // 优先级，数值越大越先执行
	Stage    Stage    `json:"stage"`    // 生命周期阶段
	Match    Match    `json:"match"`    // 匹配规则
	Actions  []Action `json:"actions"`  // 执行行为列表
}

// NewRule 创建一个新的空规则，index 为当前规则列表中的索引
func NewRule(name string, index int) Rule {
	return Rule{
		ID:       GenerateRuleID(index),
		Name:     name,
		Enabled:  true,
		Priority: 0,
		Stage:    StageRequest,
		Match:    Match{AllOf: []Condition{}, AnyOf: []Condition{}},
		Actions:  []Action{},
	}
}

// Match 匹配规则
type Match struct {
	AllOf []Condition `json:"allOf"` // AND 逻辑
	AnyOf []Condition `json:"anyOf"` // OR 逻辑
}

// ConditionType 条件类型
type ConditionType string

const (
	// URL 条件类型
	ConditionURLEquals   ConditionType = "urlEquals"   // URL 精确匹配
	ConditionURLPrefix   ConditionType = "urlPrefix"   // URL 前缀匹配
	ConditionURLSuffix   ConditionType = "urlSuffix"   // URL 后缀匹配
	ConditionURLContains ConditionType = "urlContains" // URL 包含匹配
	ConditionURLRegex    ConditionType = "urlRegex"    // URL 正则匹配

	// Method 和 ResourceType 条件类型
	ConditionMethod       ConditionType = "method"       // HTTP 方法
	ConditionResourceType ConditionType = "resourceType" // 资源类型

	// Header 条件类型
	ConditionHeaderExists    ConditionType = "headerExists"    // Header 存在
	ConditionHeaderNotExists ConditionType = "headerNotExists" // Header 不存在
	ConditionHeaderEquals    ConditionType = "headerEquals"    // Header 精确匹配
	ConditionHeaderContains  ConditionType = "headerContains"  // Header 包含
	ConditionHeaderRegex     ConditionType = "headerRegex"     // Header 正则

	// Query 条件类型
	ConditionQueryExists    ConditionType = "queryExists"    // Query 存在
	ConditionQueryNotExists ConditionType = "queryNotExists" // Query 不存在
	ConditionQueryEquals    ConditionType = "queryEquals"    // Query 精确匹配
	ConditionQueryContains  ConditionType = "queryContains"  // Query 包含
	ConditionQueryRegex     ConditionType = "queryRegex"     // Query 正则

	// Cookie 条件类型
	ConditionCookieExists    ConditionType = "cookieExists"    // Cookie 存在
	ConditionCookieNotExists ConditionType = "cookieNotExists" // Cookie 不存在
	ConditionCookieEquals    ConditionType = "cookieEquals"    // Cookie 精确匹配
	ConditionCookieContains  ConditionType = "cookieContains"  // Cookie 包含
	ConditionCookieRegex     ConditionType = "cookieRegex"     // Cookie 正则

	// Body 条件类型
	ConditionBodyContains ConditionType = "bodyContains" // Body 包含
	ConditionBodyRegex    ConditionType = "bodyRegex"    // Body 正则
	ConditionBodyJsonPath ConditionType = "bodyJsonPath" // JSON Path 匹配
)

// Condition 条件定义
type Condition struct {
	Type    ConditionType `json:"type"`              // 条件类型
	Value   string        `json:"value,omitempty"`   // 匹配值 (url*, *Equals, *Contains, bodyContains)
	Values  []string      `json:"values,omitempty"`  // 匹配值列表 (method, resourceType)
	Pattern string        `json:"pattern,omitempty"` // 正则表达式 (*Regex)
	Name    string        `json:"name,omitempty"`    // 键名 (header*, query*, cookie*)
	Path    string        `json:"path,omitempty"`    // JSON Path (bodyJsonPath)
}

// ActionType 行为类型
type ActionType string

const (
	// 请求阶段行为类型
	ActionSetUrl           ActionType = "setUrl"           // 设置请求 URL
	ActionSetMethod        ActionType = "setMethod"        // 设置请求方法
	ActionSetQueryParam    ActionType = "setQueryParam"    // 设置查询参数
	ActionRemoveQueryParam ActionType = "removeQueryParam" // 移除查询参数
	ActionSetCookie        ActionType = "setCookie"        // 设置 Cookie
	ActionRemoveCookie     ActionType = "removeCookie"     // 移除 Cookie
	ActionSetFormField     ActionType = "setFormField"     // 设置表单字段
	ActionRemoveFormField  ActionType = "removeFormField"  // 移除表单字段
	ActionBlock            ActionType = "block"            // 拦截请求

	// 请求/响应阶段通用行为类型
	ActionSetHeader       ActionType = "setHeader"       // 设置头部
	ActionRemoveHeader    ActionType = "removeHeader"    // 移除头部
	ActionSetBody         ActionType = "setBody"         // 替换 Body
	ActionAppendBody      ActionType = "appendBody"      // 追加 Body
	ActionReplaceBodyText ActionType = "replaceBodyText" // 字符串替换 Body
	ActionPatchBodyJson   ActionType = "patchBodyJson"   // JSON Patch 修改 Body

	// 响应阶段行为类型
	ActionSetStatus ActionType = "setStatus" // 设置响应状态码
)

// BodyEncoding Body 编码方式
type BodyEncoding string

const (
	BodyEncodingText   BodyEncoding = "text"   // 文本编码
	BodyEncodingBase64 BodyEncoding = "base64" // Base64 编码
)

// Action 行为定义
type Action struct {
	Type         ActionType        `json:"type"`                   // 行为类型
	Value        any               `json:"value,omitempty"`        // 目标值 (setUrl, setMethod, setStatus, setBody)
	Name         string            `json:"name,omitempty"`         // 键名 (setHeader, removeHeader, setQueryParam, setCookie, setFormField)
	Encoding     BodyEncoding      `json:"encoding,omitempty"`     // Body 编码方式 (setBody)
	Search       string            `json:"search,omitempty"`       // 搜索内容 (replaceBodyText)
	Replace      string            `json:"replace,omitempty"`      // 替换内容 (replaceBodyText)
	ReplaceAll   bool              `json:"replaceAll,omitempty"`   // 是否全部替换 (replaceBodyText)
	Patches      []JSONPatchOp     `json:"patches,omitempty"`      // JSON Patch 操作列表 (patchBodyJson)
	StatusCode   int               `json:"statusCode,omitempty"`   // HTTP 状态码 (block)
	Headers      map[string]string `json:"headers,omitempty"`      // 响应头 (block)
	Body         string            `json:"body,omitempty"`         // 响应体 (block)
	BodyEncoding BodyEncoding      `json:"bodyEncoding,omitempty"` // Body 编码方式 (block)
}

// JSONPatchOp JSON Patch 操作
type JSONPatchOp struct {
	Op    string `json:"op"`              // 操作类型: add, remove, replace, move, copy, test
	Path  string `json:"path"`            // JSON 路径
	Value any    `json:"value,omitempty"` // 值
	From  string `json:"from,omitempty"`  // 源路径 (move, copy)
}

// IsTerminal 判断行为是否为终结性行为
func (a *Action) IsTerminal() bool {
	return a.Type == ActionBlock
}

// IsValidForStage 判断行为是否适用于指定阶段
func (a *Action) IsValidForStage(stage Stage) bool {
	switch a.Type {
	// 仅请求阶段
	case ActionSetUrl, ActionSetMethod, ActionSetQueryParam, ActionRemoveQueryParam,
		ActionSetCookie, ActionRemoveCookie, ActionSetFormField, ActionRemoveFormField, ActionBlock:
		return stage == StageRequest
	// 仅响应阶段
	case ActionSetStatus:
		return stage == StageResponse
	// 两阶段通用
	case ActionSetHeader, ActionRemoveHeader, ActionSetBody, ActionAppendBody, ActionReplaceBodyText, ActionPatchBodyJson:
		return true
	default:
		return false
	}
}

// GetEncoding 获取 Body 编码方式，默认为 text
func (a *Action) GetEncoding() BodyEncoding {
	if a.Encoding == "" {
		return BodyEncodingText
	}
	return a.Encoding
}

// GetBodyEncoding 获取 block 行为的 Body 编码方式，默认为 text
func (a *Action) GetBodyEncoding() BodyEncoding {
	if a.BodyEncoding == "" {
		return BodyEncodingText
	}
	return a.BodyEncoding
}

// ResourceType 资源类型
type ResourceType string

const (
	ResourceTypeDocument   ResourceType = "document"   // HTML 文档
	ResourceTypeScript     ResourceType = "script"     // JavaScript
	ResourceTypeStylesheet ResourceType = "stylesheet" // CSS
	ResourceTypeImage      ResourceType = "image"      // 图片
	ResourceTypeMedia      ResourceType = "media"      // 音视频
	ResourceTypeFont       ResourceType = "font"       // 字体
	ResourceTypeXHR        ResourceType = "xhr"        // XMLHttpRequest
	ResourceTypeFetch      ResourceType = "fetch"      // Fetch API
	ResourceTypeWebSocket  ResourceType = "websocket"  // WebSocket
	ResourceTypeOther      ResourceType = "other"      // 其他
)
