package cdp

import (
	"context"
	"time"

	"github.com/mafredri/cdp/protocol/fetch"

	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"
)

// applyPause 进入人工审批流程并按超时默认动作处理
func (m *Manager) applyPause(ctx context.Context, ev *fetch.RequestPausedReply, p *rulespec.Pause, stage string, ruleID *model.RuleID) {
	id := string(ev.RequestID)
	ch := m.registerApproval(id)

	if !m.sendPendingItem(id, stage, ev, ruleID, ctx, p) {
		return
	}

	mut := m.waitForApproval(ch, p.TimeoutMS)
	m.applyApprovalResult(ctx, ev, mut, p, stage)
	m.unregisterApproval(id)
}

// registerApproval 注册审批通道
func (m *Manager) registerApproval(id string) chan rulespec.Rewrite {
	ch := make(chan rulespec.Rewrite, 1)
	m.approvalsMu.Lock()
	m.approvals[id] = ch
	m.approvalsMu.Unlock()
	return ch
}

// unregisterApproval 注销审批通道
func (m *Manager) unregisterApproval(id string) {
	m.approvalsMu.Lock()
	delete(m.approvals, id)
	m.approvalsMu.Unlock()
}

// waitForApproval 等待审批结果或超时，返回变更内容（nil 表示超时）
func (m *Manager) waitForApproval(ch chan rulespec.Rewrite, timeoutMS int) *rulespec.Rewrite {
	t := time.NewTimer(time.Duration(timeoutMS) * time.Millisecond)
	defer t.Stop()
	select {
	case mut := <-ch:
		return &mut
	case <-t.C:
		return nil
	}
}

// sendPendingItem 发送待审批项到 pending 通道
func (m *Manager) sendPendingItem(id, stage string, ev *fetch.RequestPausedReply, ruleID *model.RuleID, ctx context.Context, p *rulespec.Pause) bool {
	if m.pending == nil {
		return true
	}
	item := model.PendingItem{
		ID:     id,
		Stage:  stage,
		URL:    ev.Request.URL,
		Method: ev.Request.Method,
		Target: m.currentTarget,
		Rule:   ruleID,
	}
	select {
	case m.pending <- item:
		return true
	default:
		m.handlePauseOverflow(id, ctx, ev, p, stage)
		return false
	}
}

// handlePauseOverflow 处理Pause审批项超出pending队列容量的情况
func (m *Manager) handlePauseOverflow(id string, ctx context.Context, ev *fetch.RequestPausedReply, p *rulespec.Pause, stage string) {
	m.log.Warn("Pause审批项超出pending队列容量，触发降级", "id", id)
	m.applyPauseDefaultAction(ctx, ev, p, stage)
}

// applyApprovalResult 应用审批结果或超时默认动作
func (m *Manager) applyApprovalResult(ctx context.Context, ev *fetch.RequestPausedReply, mut *rulespec.Rewrite, p *rulespec.Pause, stage string) {
	if mut != nil {
		if hasEffectiveMutations(*mut) {
			m.applyRewrite(ctx, ev, mut, stage)
		} else {
			m.applyContinue(ctx, ev, stage)
		}
	} else {
		m.applyPauseDefaultAction(ctx, ev, p, stage)
	}
}

// applyPauseDefaultAction 应用Pause超时默认动作
func (m *Manager) applyPauseDefaultAction(ctx context.Context, ev *fetch.RequestPausedReply, p *rulespec.Pause, stage string) {
	switch p.DefaultAction.Type {
	case rulespec.PauseDefaultActionFulfill:
		m.applyRespond(ctx, ev, &rulespec.Respond{Status: p.DefaultAction.Status}, stage)
	case rulespec.PauseDefaultActionFail:
		m.applyFail(ctx, ev, &rulespec.Fail{Reason: p.DefaultAction.Reason})
	case rulespec.PauseDefaultActionContinueMutated:
		m.applyContinue(ctx, ev, stage)
	default:
		m.applyContinue(ctx, ev, stage)
	}
}

// hasEffectiveMutations 检查变更是否有实际效果
func hasEffectiveMutations(mut rulespec.Rewrite) bool {
	if mut.URL != nil || mut.Method != nil {
		return true
	}
	if len(mut.Headers) > 0 || len(mut.Query) > 0 || len(mut.Cookies) > 0 {
		return true
	}
	if mut.Body != nil {
		return true
	}
	return false
}
