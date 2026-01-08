package cdp

import (
	"context"
	"math/rand"
	"time"

	"github.com/mafredri/cdp/protocol/fetch"

	"cdpnetool/pkg/model"
)

// handle 处理一次拦截事件并根据规则执行相应动作
func (m *Manager) handle(ev *fetch.RequestPausedReply) {
	to := m.processTimeoutMS
	if to <= 0 {
		to = 3000
	}
	ctx, cancel := context.WithTimeout(m.ctx, time.Duration(to)*time.Millisecond)
	defer cancel()
	start := time.Now()
	m.events <- model.Event{Type: "intercepted"}
	stg := "request"
	if ev.ResponseStatusCode != nil {
		stg = "response"
	}
	m.log.Debug("开始处理拦截事件", "stage", stg, "url", ev.Request.URL, "method", ev.Request.Method)
	res := m.decide(ev, stg)
	if res == nil || res.Action == nil {
		m.applyContinue(ctx, ev, stg)
		m.log.Debug("拦截事件处理完成", "stage", stg, "duration", time.Since(start))
		return
	}
	a := res.Action
	if a.DropRate > 0 {
		if rand.Float64() < a.DropRate {
			m.applyContinue(ctx, ev, stg)
			m.events <- model.Event{Type: "degraded"}
			m.log.Warn("触发丢弃概率降级", "stage", stg)
			return
		}
	}
	if a.DelayMS > 0 {
		time.Sleep(time.Duration(a.DelayMS) * time.Millisecond)
	}
	elapsed := time.Since(start)
	if elapsed > time.Duration(to)*time.Millisecond {
		m.applyContinue(ctx, ev, stg)
		m.events <- model.Event{Type: "degraded"}
		m.log.Warn("拦截处理超时自动降级", "stage", stg, "elapsed", elapsed, "timeout", to)
		return
	}
	if a.Pause != nil {
		m.log.Info("应用暂停审批动作", "stage", stg)
		m.applyPause(ctx, ev, a.Pause, stg, res.RuleID)
		return
	}
	if a.Fail != nil {
		if m.log != nil {
			m.log.Info("apply_fail", "stage", stg)
		}
		m.applyFail(ctx, ev, a.Fail)
		m.events <- model.Event{Type: "failed", Rule: res.RuleID}
		m.log.Debug("拦截事件处理完成", "stage", stg, "duration", time.Since(start))
		return
	}
	if a.Respond != nil {
		m.log.Info("应用自定义响应动作", "stage", stg)
		m.applyRespond(ctx, ev, a.Respond, stg)
		m.events <- model.Event{Type: "fulfilled", Rule: res.RuleID}
		m.log.Debug("拦截事件处理完成", "stage", stg, "duration", time.Since(start))
		return
	}
	if a.Rewrite != nil {
		m.log.Info("应用请求响应重写动作", "stage", stg)
		m.applyRewrite(ctx, ev, a.Rewrite, stg)
		m.events <- model.Event{Type: "mutated", Rule: res.RuleID}
		m.log.Debug("拦截事件处理完成", "stage", stg, "duration", time.Since(start))
		return
	}
	m.applyContinue(ctx, ev, stg)
	m.log.Debug("拦截事件处理完成", "stage", stg, "duration", time.Since(start))
}

// dispatchPaused 根据并发配置调度单次拦截事件处理
func (m *Manager) dispatchPaused(ev *fetch.RequestPausedReply) {
	if m.pool == nil {
		go m.handle(ev)
		return
	}
	submitted := m.pool.submit(func() {
		m.handle(ev)
	})
	if !submitted {
		m.degradeAndContinue(ev, "并发队列已满")
	}
}

// consume 持续接收拦截事件并按并发限制分发处理
func (m *Manager) consume() {
	rp, err := m.client.Fetch.RequestPaused(m.ctx)
	if err != nil {
		m.log.Error("订阅拦截事件流失败", "error", err)
		m.handleStreamError(err)
		return
	}
	defer rp.Close()
	m.log.Info("开始消费拦截事件流")
	for {
		ev, err := rp.Recv()
		if err != nil {
			m.log.Error("接收拦截事件失败", "error", err)
			m.handleStreamError(err)
			return
		}
		m.dispatchPaused(ev)
	}
}

// handleStreamError 处理拦截流错误
func (m *Manager) handleStreamError(err error) {
	if m.ctx == nil {
		return
	}
	if m.ctx.Err() != nil {
		return
	}
	m.log.Warn("拦截流被中断，尝试自动重连", "error", err)
	var target model.TargetID
	if m.fixedTarget != "" {
		target = m.fixedTarget
	}
	auto := m.fixedTarget == ""
	if err := m.attachAndEnable(target, auto); err != nil {
		m.log.Error("重连附加浏览器目标失败", "error", err)
	}
}

// degradeAndContinue 统一的降级处理：直接放行请求
func (m *Manager) degradeAndContinue(ev *fetch.RequestPausedReply, reason string) {
	m.log.Warn("执行降级策略：直接放行", "reason", reason, "requestID", ev.RequestID)
	ctx, cancel := context.WithTimeout(m.ctx, 1*time.Second)
	defer cancel()
	args := &fetch.ContinueRequestArgs{RequestID: ev.RequestID}
	if err := m.client.Fetch.ContinueRequest(ctx, args); err != nil {
		m.log.Error("降级放行请求失败", "error", err)
	}
	m.events <- model.Event{Type: "degraded"}
}
