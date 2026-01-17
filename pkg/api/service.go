package api

import (
	"cdpnetool/internal/logger"
	"cdpnetool/internal/service"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"
)

// Service 服务接口
type Service interface {
	// StartSession 启动会话
	StartSession(cfg model.SessionConfig) (model.SessionID, error)

	// StopSession 停止会话
	StopSession(id model.SessionID) error

	// AttachTarget 附加目标
	AttachTarget(id model.SessionID, target model.TargetID) error

	// DetachTarget 分离目标
	DetachTarget(id model.SessionID, target model.TargetID) error

	// ListTargets 列出目标
	ListTargets(id model.SessionID) ([]model.TargetInfo, error)

	// EnableInterception 启用拦截
	EnableInterception(id model.SessionID) error

	// DisableInterception 禁用拦截
	DisableInterception(id model.SessionID) error

	// LoadRules 加载规则配置
	LoadRules(id model.SessionID, cfg *rulespec.Config) error

	// GetRuleStats 获取规则统计信息
	GetRuleStats(id model.SessionID) (model.EngineStats, error)

	// SubscribeEvents 订阅事件
	SubscribeEvents(id model.SessionID) (<-chan model.InterceptEvent, error)
}

// NewService 创建并返回服务接口实现
func NewService(l logger.Logger) Service {
	return service.New(l)
}
