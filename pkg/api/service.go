package api

import (
	logger "cdpnetool/internal/logger"
	"cdpnetool/internal/service"
	"cdpnetool/pkg/model"
)

type Service interface {
	StartSession(cfg model.SessionConfig) (model.SessionID, error)
	StopSession(id model.SessionID) error
	AttachTarget(id model.SessionID, target model.TargetID) error
	DetachTarget(id model.SessionID, target model.TargetID) error

	EnableInterception(id model.SessionID) error
	DisableInterception(id model.SessionID) error

	LoadRules(id model.SessionID, rs model.RuleSet) error
	GetRuleStats(id model.SessionID) (model.EngineStats, error)

	SubscribeEvents(id model.SessionID) (<-chan model.Event, error)
	SubscribePending(id model.SessionID) (<-chan any, error)
	ApproveRequest(itemID string, mutations model.Rewrite) error
	ApproveResponse(itemID string, mutations model.Rewrite) error
	Reject(itemID string) error
}

// NewService 创建并返回服务接口实现
func NewService() Service {
	return service.New()
}

func NewServiceWithLogger(l logger.Logger) Service { return service.NewWithLogger(l) }
