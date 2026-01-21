package cdp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mafredri/cdp/protocol/fetch"

	"cdpnetool/internal/executor"
	"cdpnetool/internal/protocol"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/domain"
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
	evalCtx := m.buildEvalContext(ev)

	// 评估匹配规则
	if m.engine == nil {
		// 无引擎，发送未匹配事件并放行
		m.sendUnmatchedEvent(ts.id, ev, stage, statusCode)
		m.executor.ContinueRequest(ctx, ts.client, ev)
		return
	}

	matchedRules := m.engine.EvalForStage(evalCtx, stage)
	if len(matchedRules) == 0 {
		// 未匹配，发送未匹配事件并放行
		m.sendUnmatchedEvent(ts.id, ev, stage, statusCode)
		if stage == rulespec.StageRequest {
			m.executor.ContinueRequest(ctx, ts.client, ev)
		} else {
			m.executor.ContinueResponse(ctx, ts.client, ev)
		}
		m.log.Debug("拦截事件处理完成，无匹配规则", "stage", stage, "duration", time.Since(start))
		return
	}

	// 有匹配规则 - 捕获原始数据
	requestInfo, responseInfo := m.captureOriginalData(ts, ev, stage)

	// 执行所有匹配规则的行为（aggregate 模式）
	if stage == rulespec.StageRequest {
		m.executeRequestStageWithTracking(ctx, ts, ev, matchedRules, requestInfo, responseInfo, start)
	} else {
		m.executeResponseStageWithTracking(ctx, ts, ev, matchedRules, requestInfo, responseInfo, start)
	}
}

// captureOriginalData 捕获原始请求/响应数据
func (m *Manager) captureOriginalData(ts *targetSession, ev *fetch.RequestPausedReply, stage rulespec.Stage) (domain.RequestInfo, domain.ResponseInfo) {
	requestInfo := domain.RequestInfo{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		Headers:      make(map[string]string),
		ResourceType: string(ev.ResourceType),
	}

	// 解析请求头
	_ = json.Unmarshal(ev.Request.Headers, &requestInfo.Headers)

	// 获取请求体
	requestInfo.Body = protocol.GetRequestBody(ev)

	// 响应信息
	responseInfo := domain.ResponseInfo{
		Headers: make(map[string]string),
	}

	if stage == rulespec.StageResponse {
		if ev.ResponseStatusCode != nil {
			responseInfo.StatusCode = *ev.ResponseStatusCode
		}
		// 响应头
		for _, h := range ev.ResponseHeaders {
			responseInfo.Headers[h.Name] = h.Value
		}
		// 响应体需要单独获取
		responseInfo.Body, _ = m.executor.FetchResponseBody(ts.ctx, ts.client, ev.RequestID)
	}

	return requestInfo, responseInfo
}

// buildRuleMatches 构建规则匹配信息列表
func buildRuleMatches(matchedRules []*rules.MatchedRule) []domain.RuleMatch {
	matches := make([]domain.RuleMatch, len(matchedRules))
	for i, mr := range matchedRules {
		// 收集实际执行的 action 类型
		actionTypes := make([]string, 0, len(mr.Rule.Actions))
		for _, action := range mr.Rule.Actions {
			actionTypes = append(actionTypes, string(action.Type))
		}
		matches[i] = domain.RuleMatch{
			RuleID:   mr.Rule.ID,
			RuleName: mr.Rule.Name,
			Actions:  actionTypes,
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
	requestInfo domain.RequestInfo,
	responseInfo domain.ResponseInfo,
	start time.Time,
) {
	var aggregatedMut *executor.RequestMutation
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
			m.executor.ApplyRequestMutation(ctx, ts.client, ev, mut)
			// 发送 blocked 事件
			m.sendMatchedEvent(ts.id, "blocked", ruleMatches, requestInfo, responseInfo)
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
	var modifiedRequestInfo domain.RequestInfo
	var modifiedResponseInfo domain.ResponseInfo

	if aggregatedMut != nil && hasRequestMutation(aggregatedMut) {
		m.executor.ApplyRequestMutation(ctx, ts.client, ev, aggregatedMut)
		finalResult = "modified"
		modifiedRequestInfo = m.captureModifiedRequestData(requestInfo, aggregatedMut)
		modifiedResponseInfo = responseInfo
	} else {
		m.executor.ContinueRequest(ctx, ts.client, ev)
		finalResult = "passed"
		modifiedRequestInfo = requestInfo
		modifiedResponseInfo = responseInfo
	}

	// 发送匹配事件
	m.sendMatchedEvent(ts.id, finalResult, ruleMatches, modifiedRequestInfo, modifiedResponseInfo)
	m.log.Debug("请求阶段处理完成", "result", finalResult, "duration", time.Since(start))
}

// executeResponseStageWithTracking 执行响应阶段的行为并跟踪变更
func (m *Manager) executeResponseStageWithTracking(
	ctx context.Context,
	ts *targetSession,
	ev *fetch.RequestPausedReply,
	matchedRules []*rules.MatchedRule,
	requestInfo domain.RequestInfo,
	responseInfo domain.ResponseInfo,
	start time.Time,
) {
	responseBody := responseInfo.Body
	var aggregatedMut *executor.ResponseMutation
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

	if aggregatedMut != nil && hasResponseMutation(aggregatedMut) {
		// 确保 Body 是最新的
		if aggregatedMut.Body == nil && responseBody != "" {
			aggregatedMut.Body = &responseBody
		}
		m.executor.ApplyResponseMutation(ctx, ts.client, ev, aggregatedMut)
		finalResult = "modified"
		modifiedResponseInfo := m.captureModifiedResponseData(responseInfo, aggregatedMut, responseBody)
		// 发送匹配事件
		m.sendMatchedEvent(ts.id, finalResult, ruleMatches, requestInfo, modifiedResponseInfo)
	} else {
		m.executor.ContinueResponse(ctx, ts.client, ev)
		finalResult = "passed"
		// 发送匹配事件
		m.sendMatchedEvent(ts.id, finalResult, ruleMatches, requestInfo, responseInfo)
	}
	m.log.Debug("响应阶段处理完成", "result", finalResult, "duration", time.Since(start))
}

// captureModifiedRequestData 捕获修改后的请求数据
func (m *Manager) captureModifiedRequestData(original domain.RequestInfo, mut *executor.RequestMutation) domain.RequestInfo {
	modified := domain.RequestInfo{
		URL:          original.URL,
		Method:       original.Method,
		ResourceType: original.ResourceType,
		Headers:      make(map[string]string),
		Body:         original.Body,
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
func (m *Manager) captureModifiedResponseData(original domain.ResponseInfo, mut *executor.ResponseMutation, finalBody string) domain.ResponseInfo {
	modified := domain.ResponseInfo{
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
func mergeRequestMutation(dst, src *executor.RequestMutation) {
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
func mergeResponseMutation(dst, src *executor.ResponseMutation) {
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
func hasRequestMutation(m *executor.RequestMutation) bool {
	return m.URL != nil || m.Method != nil ||
		len(m.Headers) > 0 || len(m.Query) > 0 || len(m.Cookies) > 0 ||
		len(m.RemoveHeaders) > 0 || len(m.RemoveQuery) > 0 || len(m.RemoveCookies) > 0 ||
		m.Body != nil
}

// hasResponseMutation 检查响应变更是否有效
func hasResponseMutation(m *executor.ResponseMutation) bool {
	return m.StatusCode != nil || len(m.Headers) > 0 || len(m.RemoveHeaders) > 0 || m.Body != nil
}

// dispatchPaused 根据并发配置调度单次拦截事件处理
func (m *Manager) dispatchPaused(ts *targetSession, ev *fetch.RequestPausedReply) {
	if m.pool == nil {
		go m.handle(ts, ev)
		return
	}
	submitted := m.pool.Submit(func() {
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
	m.executor.ContinueRequest(ctx, ts.client, ev)
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
	target domain.TargetID,
	finalResult string,
	matchedRules []domain.RuleMatch,
	requestInfo domain.RequestInfo,
	responseInfo domain.ResponseInfo,
) {
	evt := domain.InterceptEvent{
		IsMatched: true,
		Matched: &domain.MatchedEvent{
			NetworkEvent: domain.NetworkEvent{
				Session:      "", // 会在上层填充
				Target:       target,
				Timestamp:    time.Now().UnixMilli(),
				IsMatched:    true,
				Request:      requestInfo,
				Response:     responseInfo,
				FinalResult:  finalResult,
				MatchedRules: matchedRules,
			},
		},
	}

	select {
	case m.events <- evt:
	default:
	}
}

// sendUnmatchedEvent 发送未匹配事件
func (m *Manager) sendUnmatchedEvent(target domain.TargetID, ev *fetch.RequestPausedReply, stage rulespec.Stage, statusCode int) {
	requestInfo := domain.RequestInfo{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		Headers:      make(map[string]string),
		ResourceType: string(ev.ResourceType),
	}

	// 解析请求头
	_ = json.Unmarshal(ev.Request.Headers, &requestInfo.Headers)

	// 获取请求体
	requestInfo.Body = protocol.GetRequestBody(ev)

	// 响应信息
	responseInfo := domain.ResponseInfo{
		StatusCode: statusCode,
		Headers:    make(map[string]string),
	}

	if stage == rulespec.StageResponse {
		// 响应头
		for _, h := range ev.ResponseHeaders {
			responseInfo.Headers[h.Name] = h.Value
		}
		// 响应体需要单独获取（如果没有上下文则跳过）
		if ev.ResponseStatusCode != nil && len(ev.ResponseHeaders) > 0 {
			// 在未匹配的情况下，尝试获取响应体，但不依赖于ts
			// 注意：这里可能会失败，因为可能没有有效的连接来获取响应体
			responseInfo.Body = "" // 暂时设为空，因为无法在未匹配场景下获取响应体
		}
	}

	evt := domain.InterceptEvent{
		IsMatched: false,
		Unmatched: &domain.UnmatchedEvent{
			NetworkEvent: domain.NetworkEvent{
				Session:   "", // 会在上层填充
				Target:    target,
				Timestamp: time.Now().UnixMilli(),
				IsMatched: false,
				Request:   requestInfo,
				Response:  responseInfo,
			},
		},
	}

	select {
	case m.events <- evt:
	default:
	}
}
