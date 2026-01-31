package api

import (
	"context"

	"cdpnetool/internal/logger"
	"cdpnetool/internal/service"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"
)

// Service 服务接口
type Service interface {
	// StartSession 启动会话
	StartSession(ctx context.Context, cfg domain.SessionConfig) (domain.SessionID, error)

	// StopSession 停止会话
	StopSession(ctx context.Context, id domain.SessionID) error

	// AttachTarget 附加目标
	AttachTarget(ctx context.Context, id domain.SessionID, target domain.TargetID) error

	// DetachTarget 分离目标
	DetachTarget(ctx context.Context, id domain.SessionID, target domain.TargetID) error

	// ListTargets 列出目标
	ListTargets(ctx context.Context, id domain.SessionID) ([]domain.TargetInfo, error)

	// EnableInterception 启用拦截
	EnableInterception(ctx context.Context, id domain.SessionID) error

	// DisableInterception 禁用拦截
	DisableInterception(ctx context.Context, id domain.SessionID) error

	// LoadRules 加载规则配置
	LoadRules(ctx context.Context, id domain.SessionID, cfg *rulespec.Config) error

	// GetRuleStats 获取规则统计信息
	GetRuleStats(ctx context.Context, id domain.SessionID) (domain.EngineStats, error)

	// SubscribeEvents 订阅事件
	SubscribeEvents(ctx context.Context, id domain.SessionID) (<-chan domain.NetworkEvent, error)
}

// NewService 创建并返回服务接口实现
func NewService(l logger.Logger) Service {
	return service.New(l)
}
