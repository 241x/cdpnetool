package gui

import (
	"cdpnetool/internal/storage/model"
	"cdpnetool/pkg/domain"
)

// SessionData 会话数据
type SessionData struct {
	SessionID string `json:"sessionId"`
}

// TargetListData 目标列表数据
type TargetListData struct {
	Targets []domain.TargetInfo `json:"targets"`
}

// BrowserData 浏览器数据
type BrowserData struct {
	DevToolsURL string `json:"devToolsUrl"`
}

// SettingsData 设置数据
type SettingsData struct {
	Settings map[string]string `json:"settings"`
}

// SettingData 单个设置数据
type SettingData struct {
	Value string `json:"value"`
}

// ConfigData 配置数据
type ConfigData struct {
	Config *model.ConfigRecord `json:"config"`
}

// ConfigListData 配置列表数据
type ConfigListData struct {
	Configs []model.ConfigRecord `json:"configs"`
}

// NewConfigData 新配置数据
type NewConfigData struct {
	Config     *model.ConfigRecord `json:"config"`
	ConfigJSON string              `json:"configJson"`
}

// NewRuleData 新规则数据
type NewRuleData struct {
	RuleJSON string `json:"ruleJson"`
}

// StatsData 规则统计数据
type StatsData struct {
	Stats domain.EngineStats `json:"stats"`
}

// EventHistoryData 事件历史数据
type EventHistoryData struct {
	Events []model.NetworkEventRecord `json:"events"`
	Total  int64                      `json:"total"`
}
