package handler

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"cdpnetool/internal/executor"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/protocol"
	"cdpnetool/internal/rules"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/fetch"
)

// Handler 事件处理器，负责协调规则匹配、行为执行和事件发送
type Handler struct {
	engine           *rules.Engine
	executor         *executor.Executor
	events           chan domain.NetworkEvent
	processTimeoutMS int
	log              logger.Logger
}

// Config 配置选项
type Config struct {
	Engine           *rules.Engine
	Executor         *executor.Executor
	Events           chan domain.NetworkEvent
	ProcessTimeoutMS int
	Logger           logger.Logger
}

// StageContext 拦截事件阶段上下文
type StageContext struct {
	MatchedRules []*rules.MatchedRule
	RequestInfo  domain.RequestInfo
	ResponseInfo domain.ResponseInfo
	Start        time.Time
}

// New 创建事件处理器
func New(cfg Config) *Handler {
	return &Handler{
		engine:           cfg.Engine,
		executor:         cfg.Executor,
		events:           cfg.Events,
		processTimeoutMS: cfg.ProcessTimeoutMS,
		log:              cfg.Logger,
	}
}

// HandleRequest 处理请求拦截
func (h *Handler) HandleRequest(
	client *cdp.Client,
	ctx context.Context,
	targetID domain.TargetID,
	ev *fetch.RequestPausedReply,
	l logger.Logger,
) {
	l.Debug("开始处理请求拦截", "method", ev.Request.Method)

	evalCtx := h.buildEvalContext(ev)
	if h.engine == nil {
		h.sendUnmatchedRequestEvent(targetID, ev)
		h.executor.ContinueRequest(ctx, client, ev)
		return
	}

	start := time.Now()
	matchedRules := h.engine.EvalForStage(evalCtx, rulespec.StageRequest)
	if len(matchedRules) == 0 {
		h.sendUnmatchedRequestEvent(targetID, ev)
		h.executor.ContinueRequest(ctx, client, ev)
		l.Debug("请求处理完成，无匹配规则", "duration", time.Since(start))
		return
	}

	requestInfo := h.captureRequestData(ev)
	stageCtx := StageContext{
		MatchedRules: matchedRules,
		RequestInfo:  requestInfo,
		Start:        start,
	}

	h.executeRequestStageWithTracking(ctx, client, targetID, ev, stageCtx, l)
}

// HandleResponse 处理响应拦截
func (h *Handler) HandleResponse(
	client *cdp.Client,
	ctx context.Context,
	targetID domain.TargetID,
	ev *fetch.RequestPausedReply,
	l logger.Logger,
) {
	statusCode := 0
	if ev.ResponseStatusCode != nil {
		statusCode = *ev.ResponseStatusCode
	}
	l.Debug("开始处理响应拦截", "statusCode", statusCode)

	evalCtx := h.buildEvalContext(ev)
	if h.engine == nil {
		h.sendUnmatchedResponseEvent(targetID, ev, statusCode)
		h.executor.ContinueResponse(ctx, client, ev)
		return
	}

	start := time.Now()
	matchedRules := h.engine.EvalForStage(evalCtx, rulespec.StageResponse)
	if len(matchedRules) == 0 {
		h.sendUnmatchedResponseEvent(targetID, ev, statusCode)
		h.executor.ContinueResponse(ctx, client, ev)
		l.Debug("响应处理完成，无匹配规则", "duration", time.Since(start))
		return
	}

	requestInfo, responseInfo := h.captureResponseData(client, ctx, ev)
	stageCtx := StageContext{
		MatchedRules: matchedRules,
		RequestInfo:  requestInfo,
		ResponseInfo: responseInfo,
		Start:        start,
	}

	h.executeResponseStageWithTracking(ctx, client, targetID, ev, stageCtx, l)
}

// executeRequestStageWithTracking 执行请求阶段的行为并跟踪变更
func (h *Handler) executeRequestStageWithTracking(
	ctx context.Context,
	client *cdp.Client,
	targetID domain.TargetID,
	ev *fetch.RequestPausedReply,
	stageCtx StageContext,
	l logger.Logger,
) {
	var aggregatedMut *executor.RequestMutation
	ruleMatches := buildRuleMatches(stageCtx.MatchedRules)

	for _, matched := range stageCtx.MatchedRules {
		rule := matched.Rule
		if len(rule.Actions) == 0 {
			continue
		}

		// 执行当前规则的所有行为
		mut := h.executor.ExecuteRequestActions(rule.Actions, ev)
		if mut == nil {
			continue
		}

		// 检查是否是终结性行为（block）
		if mut.Block != nil {
			h.executor.ApplyRequestMutation(ctx, client, ev, mut)
			// 发送 blocked 事件
			h.sendMatchedEvent(targetID, "blocked", ruleMatches, stageCtx.RequestInfo, stageCtx.ResponseInfo)
			l.Info("请求被阻止", "rule", rule.ID)
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
		h.executor.ApplyRequestMutation(ctx, client, ev, aggregatedMut)
		finalResult = "modified"
		modifiedRequestInfo = h.captureModifiedRequestData(stageCtx.RequestInfo, aggregatedMut)
		modifiedResponseInfo = stageCtx.ResponseInfo
	} else {
		h.executor.ContinueRequest(ctx, client, ev)
		finalResult = "passed"
		modifiedRequestInfo = stageCtx.RequestInfo
		modifiedResponseInfo = stageCtx.ResponseInfo
	}

	// 发送匹配事件
	h.sendMatchedEvent(targetID, finalResult, ruleMatches, modifiedRequestInfo, modifiedResponseInfo)
	l.Debug("请求阶段处理完成", "result", finalResult, "duration", time.Since(stageCtx.Start))
}

// executeResponseStageWithTracking 执行响应阶段的行为并跟踪变更
func (h *Handler) executeResponseStageWithTracking(
	ctx context.Context,
	client *cdp.Client,
	targetID domain.TargetID,
	ev *fetch.RequestPausedReply,
	stageCtx StageContext,
	l logger.Logger,
) {
	responseBody := stageCtx.ResponseInfo.Body
	var aggregatedMut *executor.ResponseMutation
	ruleMatches := buildRuleMatches(stageCtx.MatchedRules)

	for _, matched := range stageCtx.MatchedRules {
		rule := matched.Rule
		if len(rule.Actions) == 0 {
			continue
		}

		// 执行当前规则的所有行为
		mut := h.executor.ExecuteResponseActions(rule.Actions, ev, responseBody)
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
		h.executor.ApplyResponseMutation(ctx, client, ev, aggregatedMut)
		finalResult = "modified"
		modifiedResponseInfo := h.captureModifiedResponseData(stageCtx.ResponseInfo, aggregatedMut, responseBody)
		// 发送匹配事件
		h.sendMatchedEvent(targetID, finalResult, ruleMatches, stageCtx.RequestInfo, modifiedResponseInfo)
	} else {
		h.executor.ContinueResponse(ctx, client, ev)
		finalResult = "passed"
		// 发送匹配事件
		h.sendMatchedEvent(targetID, finalResult, ruleMatches, stageCtx.RequestInfo, stageCtx.ResponseInfo)
	}
	l.Debug("响应阶段处理完成", "result", finalResult, "duration", time.Since(stageCtx.Start))
}

// SetEngine 设置规则引擎
func (h *Handler) SetEngine(engine *rules.Engine) {
	h.engine = engine
}

// SetProcessTimeout 设置处理超时时间
func (h *Handler) SetProcessTimeout(timeoutMS int) {
	h.processTimeoutMS = timeoutMS
}

// buildEvalContext 构造规则匹配上下文
func (h *Handler) buildEvalContext(ev *fetch.RequestPausedReply) *rules.EvalContext {
	headers := map[string]string{}
	query := map[string]string{}
	cookies := map[string]string{}
	var bodyText string
	var resourceType string

	if ev.ResourceType != "" {
		resourceType = string(ev.ResourceType)
	}

	_ = json.Unmarshal(ev.Request.Headers, &headers)
	if len(headers) > 0 {
		normalized := make(map[string]string, len(headers))
		for k, v := range headers {
			normalized[strings.ToLower(k)] = v
		}
		headers = normalized
	}

	if ev.Request.URL != "" {
		if u, err := url.Parse(ev.Request.URL); err == nil {
			for key, vals := range u.Query() {
				if len(vals) > 0 {
					query[strings.ToLower(key)] = vals[0]
				}
			}
		}
	}

	if v, ok := headers["cookie"]; ok {
		for name, val := range protocol.ParseCookie(v) {
			cookies[strings.ToLower(name)] = val
		}
	}

	bodyText = protocol.GetRequestBody(ev)

	return &rules.EvalContext{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		ResourceType: resourceType,
		Headers:      headers,
		Query:        query,
		Cookies:      cookies,
		Body:         bodyText,
	}
}

// sendMatchedEvent 发送匹配事件
func (h *Handler) sendMatchedEvent(
	targetID domain.TargetID,
	finalResult string,
	matchedRules []domain.RuleMatch,
	requestInfo domain.RequestInfo,
	responseInfo domain.ResponseInfo,
) {
	if h.events == nil {
		return
	}
	evt := domain.NetworkEvent{
		Session:      "", // 会在上层填充
		Target:       targetID,
		Timestamp:    time.Now().UnixMilli(),
		IsMatched:    true,
		Request:      requestInfo,
		Response:     responseInfo,
		FinalResult:  finalResult,
		MatchedRules: matchedRules,
	}

	select {
	case h.events <- evt:
	default:
	}
}

// sendUnmatchedRequestEvent 发送未匹配的请求事件
func (h *Handler) sendUnmatchedRequestEvent(targetID domain.TargetID, ev *fetch.RequestPausedReply) {
	if h.events == nil {
		return
	}

	requestInfo := domain.RequestInfo{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		Headers:      make(map[string]string),
		ResourceType: string(ev.ResourceType),
	}
	_ = json.Unmarshal(ev.Request.Headers, &requestInfo.Headers)
	requestInfo.Body = protocol.GetRequestBody(ev)

	evt := domain.NetworkEvent{
		Target:    targetID,
		Timestamp: time.Now().UnixMilli(),
		IsMatched: false,
		Request:   requestInfo,
	}

	select {
	case h.events <- evt:
	default:
	}
}

// sendUnmatchedResponseEvent 发送未匹配的响应事件
func (h *Handler) sendUnmatchedResponseEvent(targetID domain.TargetID, ev *fetch.RequestPausedReply, statusCode int) {
	if h.events == nil {
		return
	}

	requestInfo := domain.RequestInfo{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		Headers:      make(map[string]string),
		ResourceType: string(ev.ResourceType),
	}
	_ = json.Unmarshal(ev.Request.Headers, &requestInfo.Headers)
	requestInfo.Body = protocol.GetRequestBody(ev)

	responseInfo := domain.ResponseInfo{
		StatusCode: statusCode,
		Headers:    make(map[string]string),
	}
	for _, h := range ev.ResponseHeaders {
		responseInfo.Headers[h.Name] = h.Value
	}

	evt := domain.NetworkEvent{
		Target:    targetID,
		Timestamp: time.Now().UnixMilli(),
		IsMatched: false,
		Request:   requestInfo,
		Response:  responseInfo,
	}

	select {
	case h.events <- evt:
	default:
	}
}

// captureRequestData 捕获原始请求数据
func (h *Handler) captureRequestData(ev *fetch.RequestPausedReply) domain.RequestInfo {
	requestInfo := domain.RequestInfo{
		URL:          ev.Request.URL,
		Method:       ev.Request.Method,
		Headers:      make(map[string]string),
		ResourceType: string(ev.ResourceType),
	}
	_ = json.Unmarshal(ev.Request.Headers, &requestInfo.Headers)
	requestInfo.Body = protocol.GetRequestBody(ev)
	return requestInfo
}

// captureResponseData 捕获原始请求/响应数据
func (h *Handler) captureResponseData(
	client *cdp.Client,
	ctx context.Context,
	ev *fetch.RequestPausedReply,
) (domain.RequestInfo, domain.ResponseInfo) {
	requestInfo := h.captureRequestData(ev)

	responseInfo := domain.ResponseInfo{
		Headers: make(map[string]string),
	}

	if ev.ResponseStatusCode != nil {
		responseInfo.StatusCode = *ev.ResponseStatusCode
	}
	for _, h := range ev.ResponseHeaders {
		responseInfo.Headers[h.Name] = h.Value
	}
	// 响应体需要单独获取
	body, _ := h.executor.FetchResponseBody(ctx, client, ev.RequestID)
	responseInfo.Body = body

	return requestInfo, responseInfo
}

// captureModifiedRequestData 捕获修改后的请求数据
func (h *Handler) captureModifiedRequestData(original domain.RequestInfo, mut *executor.RequestMutation) domain.RequestInfo {
	modified := domain.RequestInfo{
		URL:          original.URL,
		Method:       original.Method,
		ResourceType: original.ResourceType,
		Headers:      make(map[string]string),
		Body:         original.Body,
	}

	for k, v := range original.Headers {
		modified.Headers[k] = v
	}

	if mut.URL != nil {
		modified.URL = *mut.URL
	}

	for _, h := range mut.RemoveHeaders {
		delete(modified.Headers, h)
	}
	for k, v := range mut.Headers {
		modified.Headers[k] = v
	}

	if mut.Body != nil {
		modified.Body = *mut.Body
	}

	return modified
}

// captureModifiedResponseData 捕获修改后的响应数据
func (h *Handler) captureModifiedResponseData(original domain.ResponseInfo, mut *executor.ResponseMutation, finalBody string) domain.ResponseInfo {
	modified := domain.ResponseInfo{
		StatusCode: original.StatusCode,
		Headers:    make(map[string]string),
		Body:       finalBody,
	}

	for k, v := range original.Headers {
		modified.Headers[k] = v
	}

	if mut.StatusCode != nil {
		modified.StatusCode = *mut.StatusCode
	}

	for _, h := range mut.RemoveHeaders {
		delete(modified.Headers, h)
	}
	for k, v := range mut.Headers {
		modified.Headers[k] = v
	}

	return modified
}

// buildRuleMatches 构建规则匹配信息列表
func buildRuleMatches(matchedRules []*rules.MatchedRule) []domain.RuleMatch {
	matches := make([]domain.RuleMatch, len(matchedRules))
	for i, mr := range matchedRules {
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
