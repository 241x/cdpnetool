package storage

import (
	"time"
)

// Setting 用户设置表
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// 预定义的设置 Key
const (
	SettingKeyDevToolsURL        = "devtools_url"
	SettingKeyTheme              = "theme"
	SettingKeyWindowBounds       = "window_bounds"
	SettingKeyLastRuleSetID      = "last_ruleset_id"
	SettingKeyEventRetentionDays = "event_retention_days"
)

// RuleSetRecord 规则集表
type RuleSetRecord struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;not null" json:"name"`
	Version   string    `json:"version"`
	RulesJSON string    `gorm:"type:text" json:"rulesJson"` // JSON 序列化的规则数组
	IsActive  bool      `gorm:"default:false" json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// InterceptEventRecord 拦截事件历史表
type InterceptEventRecord struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SessionID  string    `gorm:"index" json:"sessionId"`
	TargetID   string    `json:"targetId"`
	Type       string    `gorm:"index" json:"type"` // matched, rewritten, failed, rejected...
	URL        string    `json:"url"`
	Method     string    `json:"method"`
	Stage      string    `json:"stage"`
	StatusCode int       `json:"statusCode"`
	RuleID     *string   `json:"ruleId"`
	Error      string    `json:"error"`
	Timestamp  int64     `gorm:"index" json:"timestamp"`
	CreatedAt  time.Time `json:"createdAt"`
}

// RequestSnapshot 请求快照表 (P2 - 可选功能)
type RequestSnapshot struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	EventID         uint      `gorm:"index" json:"eventId"`
	RequestHeaders  string    `gorm:"type:text" json:"requestHeaders"` // JSON
	RequestBody     string    `gorm:"type:text" json:"requestBody"`
	ResponseHeaders string    `gorm:"type:text" json:"responseHeaders"` // JSON
	ResponseBody    string    `gorm:"type:text" json:"responseBody"`
	CreatedAt       time.Time `json:"createdAt"`
}
