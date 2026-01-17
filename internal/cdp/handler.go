package cdp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mafredri/cdp/protocol/fetch"

	"cdpnetool/internal/rules"
	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"
)

// handle 处理一次拦截事件并根据规则执行相应动作
func (m *Manager) handle(ts *targetSession, ev *fetch.RequestPausedReply) {
	to := m.processTimeoutMS
	if to <= 0 {
		to = 3000
	}
	ctx, cancel := context.WithTimeout(ts.ctx, time.Duration(to)*time.Millisecond)
	defer cancel()
	start := time.Now()

	// 判断阶段
	stage := rulespec.StageRequest
	statusCode := 0
	if ev.ResponseStatusCode != nil {
		stage = rulespec.StageResponse
		statusCode = *ev.ResponseStatusCode
	}

	m.log.Debug("开始处理拦截事件", "stage", stage, "url", ev.Request.URL, "method", ev.Request.Method)

	// 构建评估上下文（基于请求信息）
	evalCtx := m.buildEvalContext(ts, ev)

	// 评估匹配规则
	if m.engine == nil {
		// 无引擎，发送未匹配事件并放行
		m.sendUnmatchedEvent(ts.id, ev, stage, statusCode)
		m.executor.ContinueRequest(ctx, ts, ev)
		return
	}

	matchedRules := m.engine.EvalForStage(evalCtx, stage)
	if len(matchedRules) == 0 {
		// 未匹配，发送未匹配事件并放行
		m.sendUnmatchedEvent(ts.id, ev, stage, statusCode)
		if stage == rulespec.StageRequest {
			m.executor.ContinueRequest(ctx, ts, ev)
		} else {
			m.executor.ContinueResponse(ctx, ts, ev)
		}
		m.log.Debug("拦截事件处理完成，无匹配规则", "stage", stage, "duration", time.Since(start))
		return
	}

	// 有匹配规则 - 捕获原始数据
	original := m.captureOriginalData(ts, ev, stage)

	// 执行所有匹配规则的行为（aggregate 模式）
	if stage == rulespec.StageRequest {
		m.executeRequestStageWithTracking(ctx, ts, ev, matchedRules, original, start)
	} else {
		m.executeResponseStageWithTracking(ctx, ts, ev, matchedRules, original, start)
	}
}

// captureOriginalData 捕获原始请求/响应数据
func (m *Manager) captureOriginalData(ts *targetSession, ev *fetch.RequestPausedReply, stage rulespec.Stage) model.RequestResponseData {
	data := model.RequestResponseData{
		Headers: make(map[string]string),
	}

	if stage == rulespec.StageRequest {
		data.URL = ev.Request.URL
		// 解析请求头
		_ = json.Unmarshal(ev.Request.Headers, &data.Headers)
		// 获取请求体
		if len(ev.Request.PostDataEntries) > 0 {
			for _, entry := range ev.Request.PostDataEntries {
				if entry.Bytes != nil {
					data.Body += *entry.Bytes
				}
			}
		} else if ev.Request.PostData != nil {
			data.Body = *ev.Request.PostData
		}
	} else {
		// 响应阶段
		if ev.ResponseStatusCode != nil {
			data.StatusCode = *ev.ResponseStatusCode
		}
		// 响应头
		for _, h := range ev.ResponseHeaders {
			data.Headers[h.Name] = h.Value
		}
		// 响应体需要单独获取
		data.Body, _ = m.executor.FetchResponseBody(ts.ctx, ts, ev.RequestID)
	}

	return data
}

// buildRuleMatches 构建规则匹配信息列表
func buildRuleMatches(matchedRules []*rules.MatchedRule) []model.RuleMatch {
	matches := make([]model.RuleMatch, len(matchedRules))
	for i, mr := range matchedRules {
		matches[i] = model.RuleMatch{
			RuleID:   mr.Rule.ID,
			RuleName: mr.Rule.Name,
		}
	}
	return matches
}

// executeRequestStageWithTracking 执行请求阶段的行为并跟踪变更
func (m *Manager) executeRequestStageWithTracking(
	ctx context.Context,
	ts *targetSession,
	ev *fetch.RequestPausedReply,
	matchedRules []*rules.MatchedRule,
	original model.RequestResponseData,
	start time.Time,
) {
	var aggregatedMut *RequestMutation
	ruleMatches := buildRuleMatches(matchedRules)

	for _, matched := range matchedRules {
		rule := matched.Rule
		if len(rule.Actions) == 0 {
			continue
		}

		// 执行当前规则的所有行为
		mut := m.executor.ExecuteRequestActions(rule.Actions, ev)
		if mut == nil {
			continue
		}

		// 检查是否是终结性行为（block）
		if mut.Block != nil {
			m.executor.ApplyRequestMutation(ctx, ts, ev, mut)
			// 发送 blocked 事件
			m.sendMatchedEvent(ts.id, ev, rulespec.StageRequest, "blocked", ruleMatches, original, original)
			m.log.Info("请求被阻止", "rule", rule.ID, "url", ev.Request.URL)
			return
		}

		// 聚合变更
		if aggregatedMut == nil {
			aggregatedMut = mut
		} else {
			mergeRequestMutation(aggregatedMut, mut)
		}
	}

	// 应用聚合后的变更
	var finalResult string
	var modified model.RequestResponseData

	if aggregatedMut != nil && hasRequestMutation(aggregatedMut) {
		m.executor.ApplyRequestMutation(ctx, ts, ev, aggregatedMut)
		finalResult = "modified"
		modified = m.captureModifiedRequestData(original, aggregatedMut)
	} else {
		m.executor.ContinueRequest(ctx, ts, ev)
		finalResult = "passed"
		modified = original
	}

	// 发送匹配事件
	m.sendMatchedEvent(ts.id, ev, rulespec.StageRequest, finalResult, ruleMatches, original, modified)
	m.log.Debug("请求阶段处理完成", "result", finalResult, "duration", time.Since(start))
}

// executeResponseStageWithTracking 执行响应阶段的行为并跟踪变更
func (m *Manager) executeResponseStageWithTracking(
	ctx context.Context,
	ts *targetSession,
	ev *fetch.RequestPausedReply,
	matchedRules []*rules.MatchedRule,
	original model.RequestResponseData,
	start time.Time,
) {
	responseBody := original.Body
	var aggregatedMut *ResponseMutation
	ruleMatches := buildRuleMatches(matchedRules)

	for _, matched := range matchedRules {
		rule := matched.Rule
		if len(rule.Actions) == 0 {
			continue
		}

		// 执行当前规则的所有行为
		mut := m.executor.ExecuteResponseActions(rule.Actions, ev, responseBody)
		if mut == nil {
			continue
		}

		// 聚合变更
		if aggregatedMut == nil {
			aggregatedMut = mut
		} else {
			mergeResponseMutation(aggregatedMut, mut)
		}

		// 更新 responseBody 供后续规则使用
		if mut.Body != nil {
			responseBody = *mut.Body
		}
	}

	// 应用聚合后的变更
	var finalResult string
	var modified model.RequestResponseData

	if aggregatedMut != nil && hasResponseMutation(aggregatedMut) {
		// 确保 Body 是最新的
		if aggregatedMut.Body == nil && responseBody != "" {
			aggregatedMut.Body = &responseBody
		}
		m.executor.ApplyResponseMutation(ctx, ts, ev, aggregatedMut)
		finalResult = "modified"
		modified = m.captureModifiedResponseData(original, aggregatedMut, responseBody)
	} else {
		m.executor.ContinueResponse(ctx, ts, ev)
		finalResult = "passed"
		modified = original
	}

	// 发送匹配事件
	m.sendMatchedEvent(ts.id, ev, rulespec.StageResponse, finalResult, ruleMatches, original, modified)
	m.log.Debug("响应阶段处理完成", "result", finalResult, "duration", time.Since(start))
}

// captureModifiedRequestData 捕获修改后的请求数据
func (m *Manager) captureModifiedRequestData(original model.RequestResponseData, mut *RequestMutation) model.RequestResponseData {
	modified := model.RequestResponseData{
		URL:     original.URL,
		Headers: make(map[string]string),
		Body:    original.Body,
	}

	// 复制原始 headers
	for k, v := range original.Headers {
		modified.Headers[k] = v
	}

	// 应用 URL 修改
	if mut.URL != nil {
		modified.URL = *mut.URL
	}

	// 应用 header 修改
	for _, h := range mut.RemoveHeaders {
		delete(modified.Headers, h)
	}
	for k, v := range mut.Headers {
		modified.Headers[k] = v
	}

	// 应用 body 修改
	if mut.Body != nil {
		modified.Body = *mut.Body
	}

	return modified
}

// captureModifiedResponseData 捕获修改后的响应数据
func (m *Manager) captureModifiedResponseData(original model.RequestResponseData, mut *ResponseMutation, finalBody string) model.RequestResponseData {
	modified := model.RequestResponseData{
		StatusCode: original.StatusCode,
		Headers:    make(map[string]string),
		Body:       finalBody,
	}

	// 复制原始 headers
	for k, v := range original.Headers {
		modified.Headers[k] = v
	}

	// 应用状态码修改
	if mut.StatusCode != nil {
		modified.StatusCode = *mut.StatusCode
	}

	// 应用 header 修改
	for _, h := range mut.RemoveHeaders {
		delete(modified.Headers, h)
	}
	for k, v := range mut.Headers {
		modified.Headers[k] = v
	}

	return modified
}

// mergeRequestMutation 合并请求变更
func mergeRequestMutation(dst, src *RequestMutation) {
	if src.URL != nil {
		dst.URL = src.URL
	}
	if src.Method != nil {
		dst.Method = src.Method
	}
	for k, v := range src.Headers {
		if dst.Headers == nil {
			dst.Headers = make(map[string]string)
		}
		dst.Headers[k] = v
	}
	for k, v := range src.Query {
		if dst.Query == nil {
			dst.Query = make(map[string]string)
		}
		dst.Query[k] = v
	}
	for k, v := range src.Cookies {
		if dst.Cookies == nil {
			dst.Cookies = make(map[string]string)
		}
		dst.Cookies[k] = v
	}
	dst.RemoveHeaders = append(dst.RemoveHeaders, src.RemoveHeaders...)
	dst.RemoveQuery = append(dst.RemoveQuery, src.RemoveQuery...)
	dst.RemoveCookies = append(dst.RemoveCookies, src.RemoveCookies...)
	if src.Body != nil {
		dst.Body = src.Body
	}
}

// mergeResponseMutation 合并响应变更
func mergeResponseMutation(dst, src *ResponseMutation) {
	if src.StatusCode != nil {
		dst.StatusCode = src.StatusCode
	}
	for k, v := range src.Headers {
		if dst.Headers == nil {
			dst.Headers = make(map[string]string)
		}
		dst.Headers[k] = v
	}
	dst.RemoveHeaders = append(dst.RemoveHeaders, src.RemoveHeaders...)
	if src.Body != nil {
		dst.Body = src.Body
	}
}

// hasRequestMutation 检查请求变更是否有效
func hasRequestMutation(m *RequestMutation) bool {
	return m.URL != nil || m.Method != nil ||
		len(m.Headers) > 0 || len(m.Query) > 0 || len(m.Cookies) > 0 ||
		len(m.RemoveHeaders) > 0 || len(m.RemoveQuery) > 0 || len(m.RemoveCookies) > 0 ||
		m.Body != nil
}

// hasResponseMutation 检查响应变更是否有效
func hasResponseMutation(m *ResponseMutation) bool {
	return m.StatusCode != nil || len(m.Headers) > 0 || len(m.RemoveHeaders) > 0 || m.Body != nil
}

// dispatchPaused 根据并发配置调度单次拦截事件处理
func (m *Manager) dispatchPaused(ts *targetSession, ev *fetch.RequestPausedReply) {
	if m.pool == nil {
		go m.handle(ts, ev)
		return
	}
	submitted := m.pool.submit(func() {
		m.handle(ts, ev)
	})
	if !submitted {
		m.degradeAndContinue(ts, ev, "并发队列已满")
	}
}

// consume 持续接收拦截事件并按并发限制分发处理
func (m *Manager) consume(ts *targetSession) {
	rp, err := ts.client.Fetch.RequestPaused(ts.ctx)
	if err != nil {
		m.log.Err(err, "订阅拦截事件流失败", "target", string(ts.id))
		m.handleTargetStreamClosed(ts, err)
		return
	}
	defer rp.Close()

	m.log.Info("开始消费拦截事件流", "target", string(ts.id))
	for {
		ev, err := rp.Recv()
		if err != nil {
			m.log.Err(err, "接收拦截事件失败", "target", string(ts.id))
			m.handleTargetStreamClosed(ts, err)
			return
		}
		m.dispatchPaused(ts, ev)
	}
}

// handleTargetStreamClosed 处理单个目标的拦截流终止
func (m *Manager) handleTargetStreamClosed(ts *targetSession, err error) {
	if !m.isEnabled() {
		m.log.Info("拦截已禁用，停止目标事件消费", "target", string(ts.id))
		return
	}

	m.log.Warn("拦截流被中断，自动移除目标", "target", string(ts.id), "error", err)

	m.targetsMu.Lock()
	defer m.targetsMu.Unlock()

	if cur, ok := m.targets[ts.id]; ok && cur == ts {
		m.closeTargetSession(cur)
		delete(m.targets, ts.id)
	}
}

// degradeAndContinue 统一的降级处理：直接放行请求
func (m *Manager) degradeAndContinue(ts *targetSession, ev *fetch.RequestPausedReply, reason string) {
	m.log.Warn("执行降级策略：直接放行", "target", string(ts.id), "reason", reason, "requestID", ev.RequestID)
	ctx, cancel := context.WithTimeout(ts.ctx, 1*time.Second)
	defer cancel()
	m.executor.ContinueRequest(ctx, ts, ev)
	// 降级时发送未匹配事件
	stage := rulespec.StageRequest
	statusCode := 0
	if ev.ResponseStatusCode != nil {
		stage = rulespec.StageResponse
		statusCode = *ev.ResponseStatusCode
	}
	m.sendUnmatchedEvent(ts.id, ev, stage, statusCode)
}

// sendMatchedEvent 发送匹配事件
func (m *Manager) sendMatchedEvent(
	target model.TargetID,
	ev *fetch.RequestPausedReply,
	stage rulespec.Stage,
	finalResult string,
	matchedRules []model.RuleMatch,
	original, modified model.RequestResponseData,
) {
	statusCode := 0
	if ev.ResponseStatusCode != nil {
		statusCode = *ev.ResponseStatusCode
	}

	evt := model.InterceptEvent{
		IsMatched: true,
		Matched: &model.MatchedEvent{
			Target:       target,
			URL:          ev.Request.URL,
			Method:       ev.Request.Method,
			Stage:        string(stage),
			StatusCode:   statusCode,
			Timestamp:    time.Now().UnixMilli(),
			FinalResult:  finalResult,
			MatchedRules: matchedRules,
			Original:     original,
			Modified:     modified,
		},
	}

	select {
	case m.events <- evt:
	default:
	}
}

// sendUnmatchedEvent 发送未匹配事件
func (m *Manager) sendUnmatchedEvent(target model.TargetID, ev *fetch.RequestPausedReply, stage rulespec.Stage, statusCode int) {
	evt := model.InterceptEvent{
		IsMatched: false,
		Unmatched: &model.UnmatchedEvent{
			Target:     target,
			URL:        ev.Request.URL,
			Method:     ev.Request.Method,
			Stage:      string(stage),
			StatusCode: statusCode,
			Timestamp:  time.Now().UnixMilli(),
		},
	}

	select {
	case m.events <- evt:
	default:
	}
}

// getStatusCode 获取响应状态码
func getStatusCode(ev *fetch.RequestPausedReply) int {
	if ev.ResponseStatusCode != nil {
		return *ev.ResponseStatusCode
	}
	return 0
}
