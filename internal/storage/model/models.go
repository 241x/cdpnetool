package model

import (
	"time"
)

// Setting 用户设置表
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`  // 设置键
	Value     string    `gorm:"type:text" json:"value"` // 设置值
	UpdatedAt time.Time `json:"updatedAt"`              // 更新时间
}

// 预定义的设置 Key
const (
	SettingKeyDevToolsURL  = "devtools_url"   // 开发者工具URL
	SettingKeyTheme        = "theme"          // 主题
	SettingKeyWindowBounds = "window_bounds"  // 窗口大小和位置
	SettingKeyLastConfigID = "last_config_id" // 上次使用的配置 ID
)

// ConfigRecord 配置表（存储规则配置）
type ConfigRecord struct {
	ID         uint      `gorm:"primaryKey" json:"id"`                 // 数据库主键（内部使用）
	ConfigID   string    `gorm:"uniqueIndex;not null" json:"configId"` // 配置业务ID（唯一索引）
	Name       string    `gorm:"not null" json:"name"`                 // 配置名称
	Version    string    `json:"version"`                              // 配置格式版本
	ConfigJSON string    `gorm:"type:text" json:"configJson"`          // 完整配置 JSON
	IsActive   bool      `gorm:"default:false" json:"isActive"`        // 是否为激活配置
	CreatedAt  time.Time `json:"createdAt"`                            // 创建时间
	UpdatedAt  time.Time `json:"updatedAt"`                            // 更新时间
}

// NetworkEventRecord 网络事件记录表（存储匹配的请求）
type NetworkEventRecord struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	SessionID        string    `gorm:"index" json:"sessionId"`
	TargetID         string    `json:"targetId"`
	URL              string    `json:"url"`
	Method           string    `json:"method"`
	StatusCode       int       `json:"statusCode"`                        // 状态码
	FinalResult      string    `gorm:"index" json:"finalResult"`          // blocked / modified / passed
	MatchedRulesJSON string    `gorm:"type:text" json:"matchedRulesJson"` // 匹配规则 JSON 数组
	RequestJSON      string    `gorm:"type:text" json:"requestJson"`      // 请求信息 JSON
	ResponseJSON     string    `gorm:"type:text" json:"responseJson"`     // 响应信息 JSON
	Timestamp        int64     `gorm:"index" json:"timestamp"`
	CreatedAt        time.Time `json:"createdAt"`
}

// TableName 指定表名（保持与旧表名一致，避免迁移）
func (NetworkEventRecord) TableName() string {
	return "matched_event_records"
}
