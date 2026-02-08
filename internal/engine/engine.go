package engine

import (
	"sort"
	"strings"
	"sync"

	"cdpnetool/internal/regexutil"
	"cdpnetool/pkg/domain"
	"cdpnetool/pkg/rulespec"

	"github.com/tidwall/gjson"
)

// MatchedRule 匹配成功的规则及其详细信息
type MatchedRule struct {
	Rule *rulespec.Rule
}

// Engine 规则决策引擎
type Engine struct {
	config  *rulespec.Config
	mu      sync.RWMutex
	total   int64
	matched int64
	byRule  map[string]int64
	cache   *regexutil.Cache
}

// New 创建一个新的规则引擎实例
func New(config *rulespec.Config) *Engine {
	return &Engine{
		config: config,
		byRule: make(map[string]int64),
		cache:  regexutil.New(),
	}
}

// Update 更新规则配置
func (e *Engine) Update(config *rulespec.Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = config
}

// Eval 评估请求并返回匹配的规则列表 (按优先级降序)
func (e *Engine) Eval(req *domain.Request, stage rulespec.Stage) []*MatchedRule {
	e.mu.RLock()
	config := e.config
	e.mu.RUnlock()

	if config == nil || len(config.Rules) == 0 {
		return nil
	}

	var matched []*MatchedRule
	for i := range config.Rules {
		rule := &config.Rules[i]
		if !rule.Enabled || rule.Stage != stage {
			continue
		}

		if e.matchRule(req, &rule.Match) {
			matched = append(matched, &MatchedRule{Rule: rule})
		}
	}

	if len(matched) == 0 {
		return nil
	}

	// 按优先级从大到小排序
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Rule.Priority > matched[j].Rule.Priority
	})

	return matched
}

// RecordStats 记录匹配统计信息
func (e *Engine) RecordStats(matched []*MatchedRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.total++
	if len(matched) > 0 {
		e.matched++
		for _, m := range matched {
			e.byRule[m.Rule.ID]++
		}
	}
}

// matchRule 评估单个规则的匹配条件
func (e *Engine) matchRule(req *domain.Request, m *rulespec.Match) bool {
	// allOf: 必须全部满足
	if len(m.AllOf) > 0 {
		for i := range m.AllOf {
			if !e.evalCondition(req, &m.AllOf[i]) {
				return false
			}
		}
	}
	// anyOf: 满足任一即可
	if len(m.AnyOf) > 0 {
		anyMatch := false
		for i := range m.AnyOf {
			if e.evalCondition(req, &m.AnyOf[i]) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			return false
		}
	}
	return true
}

// evalCondition 评估单个条件
func (e *Engine) evalCondition(req *domain.Request, c *rulespec.Condition) bool {
	switch c.Type {
	case rulespec.ConditionURLEquals:
		return req.URL == c.Value
	case rulespec.ConditionURLPrefix:
		return strings.HasPrefix(req.URL, c.Value)
	case rulespec.ConditionURLSuffix:
		return strings.HasSuffix(req.URL, c.Value)
	case rulespec.ConditionURLContains:
		return strings.Contains(req.URL, c.Value)
	case rulespec.ConditionURLRegex:
		return e.matchRegex(req.URL, c.Pattern)

	case rulespec.ConditionMethod:
		for _, v := range c.Values {
			if strings.EqualFold(req.Method, v) {
				return true
			}
		}
		return false

	case rulespec.ConditionResourceType:
		// 直接比较 ResourceType（已经是规范化的小写字符串）
		for _, v := range c.Values {
			if string(req.ResourceType) == v {
				return true
			}
		}
		return false

	case rulespec.ConditionHeaderExists:
		return req.Headers.Get(c.Name) != ""
	case rulespec.ConditionHeaderNotExists:
		return req.Headers.Get(c.Name) == ""
	case rulespec.ConditionHeaderEquals:
		return req.Headers.Get(c.Name) == c.Value
	case rulespec.ConditionHeaderContains:
		return strings.Contains(req.Headers.Get(c.Name), c.Value)
	case rulespec.ConditionHeaderRegex:
		return e.matchRegex(req.Headers.Get(c.Name), c.Pattern)

	case rulespec.ConditionQueryExists:
		_, ok := req.Query[c.Name]
		return ok
	case rulespec.ConditionQueryNotExists:
		_, ok := req.Query[c.Name]
		return !ok
	case rulespec.ConditionQueryEquals:
		v, ok := req.Query[c.Name]
		return ok && v == c.Value
	case rulespec.ConditionQueryContains:
		v, ok := req.Query[c.Name]
		return ok && strings.Contains(v, c.Value)
	case rulespec.ConditionQueryRegex:
		v, ok := req.Query[c.Name]
		return ok && e.matchRegex(v, c.Pattern)

	case rulespec.ConditionCookieExists:
		_, ok := req.Cookies[c.Name]
		return ok
	case rulespec.ConditionCookieNotExists:
		_, ok := req.Cookies[c.Name]
		return !ok
	case rulespec.ConditionCookieEquals:
		v, ok := req.Cookies[c.Name]
		return ok && v == c.Value
	case rulespec.ConditionCookieContains:
		v, ok := req.Cookies[c.Name]
		return ok && strings.Contains(v, c.Value)
	case rulespec.ConditionCookieRegex:
		v, ok := req.Cookies[c.Name]
		return ok && e.matchRegex(v, c.Pattern)

	case rulespec.ConditionBodyContains:
		return strings.Contains(string(req.Body), c.Value)
	case rulespec.ConditionBodyRegex:
		return e.matchRegex(string(req.Body), c.Pattern)
	case rulespec.ConditionBodyJsonPath:
		val, ok := e.evalJsonPath(string(req.Body), c.Path)
		return ok && val == c.Value

	default:
		return false
	}
}

// evalJsonPath 评估 JSON Path 表达式
func (e *Engine) evalJsonPath(body, path string) (string, bool) {
	if body == "" || path == "" {
		return "", false
	}
	searchPath := path
	if strings.HasPrefix(path, "$.") {
		searchPath = path[2:]
	}
	result := gjson.Get(body, searchPath)
	if !result.Exists() {
		return "", false
	}
	return result.String(), true
}

// matchRegex 正则匹配，使用缓存提升性能
func (e *Engine) matchRegex(s, pattern string) bool {
	re, err := e.cache.Get(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

// GetStats 获取统计信息
func (e *Engine) GetStats() (int64, int64, map[string]int64) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	byRule := make(map[string]int64, len(e.byRule))
	for k, v := range e.byRule {
		byRule[k] = v
	}
	return e.total, e.matched, byRule
}
