package processor

import (
	"context"
	"net/url"
	"strings"

	"cdpnetool/internal/auditor"
	"cdpnetool/internal/engine"
	"cdpnetool/internal/logger"
	"cdpnetool/internal/tracker"
	"cdpnetool/internal/transformer"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"
)

// Result 处理结果
type Result struct {
	Action      Action           // 动作：放行、修改、拦截
	ModifiedReq *domain.Request  // 修改后的请求
	ModifiedRes *domain.Response // 修改后的响应
	MockRes     *domain.Response // 伪造的响应
}

type Action string

const (
	ActionPass   Action = "pass"
	ActionModify Action = "modify"
	ActionBlock  Action = "block"
)

// PendingState 暂存在 tracker 中的请求上下文
type PendingState struct {
	Request      *domain.Request
	MatchedRules []*engine.MatchedRule
	IsModified   bool
}

// Processor 业务处理编排中心
type Processor struct {
	tracker        *tracker.Tracker
	engine         *engine.Engine
	matchedAuditor *auditor.Auditor // 匹配事件审计器
	trafficAuditor *auditor.Auditor // 全量流量审计器
	log            logger.Logger
	sessionID      string // 会话ID
	targetID       string // 目标ID
}

// New 创建一个新的处理器
func New(t *tracker.Tracker, e *engine.Engine, matchedAud, trafficAud *auditor.Auditor, l logger.Logger) *Processor {
	if l == nil {
		l = logger.NewNop()
	}
	return &Processor{
		tracker:        t,
		engine:         e,
		matchedAuditor: matchedAud,
		trafficAuditor: trafficAud,
		log:            l,
	}
}

// SetContext 设置会话和目标上下文
func (p *Processor) SetContext(sessionID, targetID string) {
	p.sessionID = sessionID
	p.targetID = targetID
}

// ProcessRequest 处理请求阶段逻辑
func (p *Processor) ProcessRequest(ctx context.Context, req *domain.Request) Result {
	p.log.Debug("[Processor] 开始处理请求", "requestID", req.ID, "url", req.URL, "method", req.Method)

	matched := p.engine.Eval(req, rulespec.StageRequest)
	p.engine.RecordStats(matched)

	// 记录匹配情况
	if len(matched) == 0 {
		p.log.Debug("[Processor] 请求未匹配规则", "requestID", req.ID)
	} else {
		ruleIDs := make([]string, len(matched))
		for i, m := range matched {
			ruleIDs[i] = m.Rule.ID
		}
		p.log.Debug("[Processor] 请求匹配规则", "requestID", req.ID, "matchedCount", len(matched), "ruleIDs", ruleIDs)
	}

	res := Result{Action: ActionPass}
	isModified := false

	for _, mr := range matched {
		for _, action := range mr.Rule.Actions {
			if action.Type == rulespec.ActionBlock {
				p.log.Info("[Processor] 执行 Block 动作", "requestID", req.ID, "ruleID", mr.Rule.ID, "statusCode", action.StatusCode)
				res.Action = ActionBlock
				res.MockRes = domain.NewResponse()
				res.MockRes.StatusCode = action.StatusCode
				if action.Body != "" {
					body, err := transformer.DecodeBody(action.Body, action.GetBodyEncoding())
					if err != nil {
						p.log.Err(err, "Block 动作中响应体解码失败", "requestID", req.ID)
						res.MockRes.Body = []byte(action.Body)
					} else {
						res.MockRes.Body = []byte(body)
					}
				}
				res.MockRes.Headers = make(domain.Header)
				for k, v := range action.Headers {
					res.MockRes.Headers.Set(k, v)
				}

				// Block 动作需立即记录审计（响应阶段不会再执行）
				// 1. 全量流量审计
				p.trafficAuditor.Record(p.sessionID, p.targetID, req, res.MockRes, "blocked", p.toRuleMatches(matched))
				// 2. 匹配事件审计（仅匹配时记录）
				if len(matched) > 0 {
					p.matchedAuditor.Record(p.sessionID, p.targetID, req, res.MockRes, "blocked", p.toRuleMatches(matched))
				}
				p.log.Debug("[Processor] Block 执行完成", "requestID", req.ID)
				return res
			}

			p.applyRequestAction(req, action)
			isModified = true
		}
	}

	if isModified {
		// 重建 URL（如果 Query 参数被修改）
		rebuildURLFromQuery(req)
		// 重建 Cookie Header（如果 Cookies 被修改）
		if cookieStr := transformer.BuildCookieString(req.Cookies); cookieStr != "" {
			req.Headers.Set("Cookie", cookieStr)
		} else {
			req.Headers.Del("Cookie")
		}

		res.Action = ActionModify
		res.ModifiedReq = req
		p.log.Debug("[Processor] 请求已修改", "requestID", req.ID, "matchedCount", len(matched))
	}

	p.tracker.Set(req.ID, &PendingState{
		Request:      req,
		MatchedRules: matched,
		IsModified:   isModified,
	})
	p.log.Debug("[Processor] 请求已入池", "requestID", req.ID)

	return res
}

// ProcessResponse 处理响应阶段逻辑
func (p *Processor) ProcessResponse(ctx context.Context, reqID string, res *domain.Response) Result {
	p.log.Debug("[Processor] 开始处理响应", "requestID", reqID, "statusCode", res.StatusCode)

	stateVal, ok := p.tracker.Get(reqID)
	if !ok {
		p.log.Warn("[Processor] 响应未找到对应请求", "requestID", reqID)
		return Result{Action: ActionPass}
	}
	state := stateVal.(*PendingState)
	p.log.Debug("[Processor] 从池中获取请求", "requestID", reqID, "url", state.Request.URL)

	matched := p.engine.Eval(state.Request, rulespec.StageResponse)
	p.engine.RecordStats(matched)

	if len(matched) > 0 {
		ruleIDs := make([]string, len(matched))
		for i, m := range matched {
			ruleIDs[i] = m.Rule.ID
		}
		p.log.Debug("[Processor] 响应匹配规则", "requestID", reqID, "matchedCount", len(matched), "ruleIDs", ruleIDs)
	}

	finalResult := "passed"
	if state.IsMatched() {
		finalResult = "matched"
	}
	if state.IsModified {
		finalResult = "modified"
	}

	if len(matched) > 0 {
		for _, mr := range matched {
			for _, action := range mr.Rule.Actions {
				p.applyResponseAction(res, action, reqID)
				finalResult = "modified"
			}
		}
	}

	allMatched := append(state.MatchedRules, matched...)
	ruleMatches := p.toRuleMatches(allMatched)

	// 1. 全量流量审计
	p.trafficAuditor.Record(p.sessionID, p.targetID, state.Request, res, finalResult, ruleMatches)
	// 2. 匹配事件审计（仅匹配时记录）
	if len(allMatched) > 0 {
		p.matchedAuditor.Record(p.sessionID, p.targetID, state.Request, res, finalResult, ruleMatches)
	}
	p.log.Debug("[Processor] 响应处理完成", "requestID", reqID, "finalResult", finalResult)

	if finalResult == "modified" {
		return Result{
			Action:      ActionModify,
			ModifiedRes: res,
		}
	}
	return Result{Action: ActionPass}
}

// toRuleMatches 将内部匹配结果转换为领域模型
func (p *Processor) toRuleMatches(matched []*engine.MatchedRule) []domain.RuleMatch {
	res := make([]domain.RuleMatch, len(matched))
	for i, m := range matched {
		actions := make([]string, len(m.Rule.Actions))
		for j, action := range m.Rule.Actions {
			actions[j] = string(action.Type)
		}
		res[i] = domain.RuleMatch{
			RuleID:   m.Rule.ID,
			RuleName: m.Rule.Name,
			Actions:  actions,
		}
	}
	return res
}

// applyRequestAction 应用单个请求修改动作
func (p *Processor) applyRequestAction(req *domain.Request, action rulespec.Action) {
	p.log.Debug("[Processor] 应用请求修改", "requestID", req.ID, "actionType", action.Type, "actionName", action.Name)
	switch action.Type {
	case rulespec.ActionSetUrl:
		if v, ok := action.Value.(string); ok {
			req.URL = v
		}
	case rulespec.ActionSetMethod:
		if v, ok := action.Value.(string); ok {
			req.Method = v
		}
	case rulespec.ActionSetHeader:
		if v, ok := action.Value.(string); ok {
			req.Headers.Set(action.Name, v)
		}
	case rulespec.ActionRemoveHeader:
		req.Headers.Del(action.Name)
	case rulespec.ActionSetQueryParam:
		if v, ok := action.Value.(string); ok {
			req.Query[action.Name] = v
		}
	case rulespec.ActionRemoveQueryParam:
		delete(req.Query, action.Name)
	case rulespec.ActionSetCookie:
		if v, ok := action.Value.(string); ok {
			req.Cookies[action.Name] = v
		}
	case rulespec.ActionRemoveCookie:
		delete(req.Cookies, action.Name)
	case rulespec.ActionSetBody:
		if v, ok := action.Value.(string); ok {
			body, err := transformer.DecodeBody(v, action.GetEncoding())
			if err != nil {
				p.log.Err(err, "请求体解码失败", "requestID", req.ID)
			} else {
				req.Body = []byte(body)
			}
		}
	case rulespec.ActionAppendBody:
		if v, ok := action.Value.(string); ok {
			appendText, err := transformer.DecodeBody(v, action.GetEncoding())
			if err != nil {
				p.log.Err(err, "追加请求体解码失败", "requestID", req.ID)
			} else {
				req.Body = append(req.Body, []byte(appendText)...)
			}
		}
	case rulespec.ActionReplaceBodyText:
		newBody := transformer.ReplaceText(string(req.Body), action.Search, action.Replace, action.ReplaceAll)
		req.Body = []byte(newBody)
	case rulespec.ActionPatchBodyJson:
		newBody, err := transformer.PatchJSON(string(req.Body), action.Patches)
		if err != nil {
			p.log.Err(err, "请求体 JSON Patch 失败", "requestID", req.ID)
		} else {
			req.Body = []byte(newBody)
		}
	case rulespec.ActionSetFormField:
		if v, ok := action.Value.(string); ok {
			newBody, err := transformer.SetFormUrlencoded(string(req.Body), action.Name, v)
			if err != nil {
				p.log.Err(err, "设置表单字段失败", "requestID", req.ID)
			} else {
				req.Body = []byte(newBody)
			}
		}
	case rulespec.ActionRemoveFormField:
		newBody, err := transformer.RemoveFormUrlencoded(string(req.Body), action.Name)
		if err != nil {
			p.log.Err(err, "移除表单字段失败", "requestID", req.ID)
		} else {
			req.Body = []byte(newBody)
		}
	}
}

// applyResponseAction 应用单个响应修改动作
func (p *Processor) applyResponseAction(res *domain.Response, action rulespec.Action, reqID string) {
	p.log.Debug("[Processor] 应用响应修改", "requestID", reqID, "actionType", action.Type, "actionName", action.Name)
	switch action.Type {
	case rulespec.ActionSetStatus:
		if v, ok := action.Value.(float64); ok {
			res.StatusCode = int(v)
		} else if v, ok := action.Value.(int); ok {
			res.StatusCode = v
		}
	case rulespec.ActionSetHeader:
		if v, ok := action.Value.(string); ok {
			res.Headers.Set(action.Name, v)
		}
	case rulespec.ActionRemoveHeader:
		res.Headers.Del(action.Name)
	case rulespec.ActionSetBody:
		if v, ok := action.Value.(string); ok {
			body, err := transformer.DecodeBody(v, action.GetEncoding())
			if err != nil {
				p.log.Err(err, "响应体解码失败", "requestID", reqID)
			} else {
				res.Body = []byte(body)
			}
		}
	case rulespec.ActionAppendBody:
		if v, ok := action.Value.(string); ok {
			appendText, err := transformer.DecodeBody(v, action.GetEncoding())
			if err != nil {
				p.log.Err(err, "追加响应体解码失败", "requestID", reqID)
			} else {
				res.Body = append(res.Body, []byte(appendText)...)
			}
		}
	case rulespec.ActionReplaceBodyText:
		newBody := transformer.ReplaceText(string(res.Body), action.Search, action.Replace, action.ReplaceAll)
		res.Body = []byte(newBody)
	case rulespec.ActionPatchBodyJson:
		newBody, err := transformer.PatchJSON(string(res.Body), action.Patches)
		if err != nil {
			p.log.Err(err, "响应体 JSON Patch 失败", "requestID", reqID)
		} else {
			res.Body = []byte(newBody)
		}
	}
}

// IsMatched 判断请求是否匹配了任何规则
func (s *PendingState) IsMatched() bool {
	return len(s.MatchedRules) > 0
}

// rebuildURLFromQuery 从 Query 字典重建 URL 的查询参数部分
func rebuildURLFromQuery(req *domain.Request) {
	if len(req.Query) == 0 {
		// 如果 Query 为空，移除 URL 中的查询参数
		if idx := strings.Index(req.URL, "?"); idx != -1 {
			req.URL = req.URL[:idx]
		}
		return
	}

	// 解析基础 URL
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		// 解析失败，保持原 URL 不变
		return
	}

	// 从 Query 字典构建查询参数
	query := url.Values{}
	for k, v := range req.Query {
		query.Set(k, v)
	}

	// 更新 URL 的查询参数
	parsedURL.RawQuery = query.Encode()
	req.URL = parsedURL.String()
}
