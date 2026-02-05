package auditor

import (
	"time"

	"cdpnetool/internal/logger"
	"cdpnetool/pkg/domain"
)

// Auditor 审计与观察者，负责流量快照的记录、持久化与分发
type Auditor struct {
	enabled bool
	events  chan domain.NetworkEvent
	log     logger.Logger
}

// New 创建一个新的审计员
func New(events chan domain.NetworkEvent, l logger.Logger) *Auditor {
	if l == nil {
		l = logger.NewNop()
	}
	return &Auditor{
		enabled: true,
		events:  events,
		log:     l,
	}
}

// SetEnabled 设置是否启用审计
func (a *Auditor) SetEnabled(enabled bool) {
	a.enabled = enabled
}

// IsEnabled 获取审计启用状态
func (a *Auditor) IsEnabled() bool {
	return a.enabled
}

// Record 记录一个完整的流量事件
func (a *Auditor) Record(
	sessionID string,
	targetID string,
	req *domain.Request,
	res *domain.Response,
	result string,
	matchedRules []domain.RuleMatch,
) {
	if !a.enabled || req == nil {
		if !a.enabled && req != nil {
			a.log.Debug("[Auditor] 审计已禁用，跳过记录", "requestID", req.ID)
		}
		return
	}

	a.log.Debug("[Auditor] 开始记录事件", "requestID", req.ID, "result", result, "matchedRules", len(matchedRules))

	evt := domain.NetworkEvent{
		ID:           req.ID,
		Session:      domain.SessionID(sessionID),
		Target:       domain.TargetID(targetID),
		Timestamp:    time.Now().UnixMilli(),
		IsMatched:    len(matchedRules) > 0,
		FinalResult:  result,
		MatchedRules: matchedRules,
		Request:      *req,
		Response:     res,
	}

	a.dispatch(evt)
	a.log.Debug("[Auditor] 事件记录完成", "requestID", req.ID)
}

// dispatch 分发事件到实时观察通道，通道满时丢弃
func (a *Auditor) dispatch(evt domain.NetworkEvent) {
	if a.events == nil {
		a.log.Debug("[Auditor] 事件通道为 nil，跳过分发", "requestID", evt.ID)
		return
	}

	select {
	case a.events <- evt:
		a.log.Debug("[Auditor] 事件分发成功", "requestID", evt.ID)
	default:
		// 通道满时丢弃，防止阻塞主流程
		a.log.Warn("[Auditor] 审计事件分发通道已满，丢弃事件", "id", evt.ID)
	}
}
