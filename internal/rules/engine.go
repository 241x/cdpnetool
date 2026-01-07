package rules

import (
	"encoding/json"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"cdpnetool/pkg/model"
	"cdpnetool/pkg/rulespec"
)

type Engine struct {
	rs      rulespec.RuleSet
	mu      sync.RWMutex
	total   int64
	matched int64
	byRule  map[model.RuleID]int64
}

// New 创建规则引擎并加载规则集
func New(rs rulespec.RuleSet) *Engine {
	return &Engine{rs: rs}
}

// Update 更新引擎内的规则集
func (e *Engine) Update(rs rulespec.RuleSet) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rs = rs
}

type Ctx struct {
	URL         string
	Method      string
	Headers     map[string]string
	Query       map[string]string
	Cookies     map[string]string
	Body        string
	ContentType string
	Stage       string
}

type Result struct {
	RuleID *model.RuleID
	Action *rulespec.Action
}

// Eval 评估一次拦截上下文并返回命中的规则与动作
func (e *Engine) Eval(ctx Ctx) *Result {
	e.mu.Lock()
	e.total++
	rs := e.rs
	e.mu.Unlock()
	if len(rs.Rules) == 0 {
		return nil
	}
	var chosen *rulespec.Rule
	for i := range rs.Rules {
		r := &rs.Rules[i]
		if matchRule(ctx, r.Match) {
			if chosen == nil || r.Priority > chosen.Priority {
				chosen = r
				if r.Mode == "short_circuit" {
					break
				}
			}
		}
	}
	if chosen == nil {
		return nil
	}
	e.mu.Lock()
	e.matched++
	if e.byRule == nil {
		e.byRule = make(map[model.RuleID]int64)
	}
	e.byRule[chosen.ID] = e.byRule[chosen.ID] + 1
	e.mu.Unlock()
	rid := chosen.ID
	return &Result{RuleID: &rid, Action: &chosen.Action}
}

// matchRule 按All/Any/None组合逻辑判断是否匹配
func matchRule(ctx Ctx, m rulespec.Match) bool {
	ok := true
	if len(m.AllOf) > 0 {
		ok = ok && allOf(ctx, m.AllOf)
	}
	if len(m.AnyOf) > 0 {
		ok = ok && anyOf(ctx, m.AnyOf)
	}
	if len(m.NoneOf) > 0 {
		ok = ok && noneOf(ctx, m.NoneOf)
	}
	return ok
}

// allOf 所有条件需满足
func allOf(ctx Ctx, cs []rulespec.Condition) bool {
	for i := range cs {
		if !cond(ctx, cs[i]) {
			return false
		}
	}
	return true
}

// anyOf 任一条件满足即可
func anyOf(ctx Ctx, cs []rulespec.Condition) bool {
	for i := range cs {
		if cond(ctx, cs[i]) {
			return true
		}
	}
	return false
}

// noneOf 所有条件均不应满足
func noneOf(ctx Ctx, cs []rulespec.Condition) bool { return !anyOf(ctx, cs) }

// cond 评估单个条件是否命中
func cond(ctx Ctx, c rulespec.Condition) bool {
	switch c.Type {
	case rulespec.ConditionTypeURL:
		switch c.Mode {
		case rulespec.ConditionModePrefix:
			return strings.HasPrefix(ctx.URL, c.Pattern)
		case rulespec.ConditionModeRegex:
			return matchRegex(ctx.URL, c.Pattern)
		case rulespec.ConditionModeExact:
			return ctx.URL == c.Pattern
		default:
			return glob(ctx.URL, c.Pattern)
		}
	case rulespec.ConditionTypeMethod:
		for _, v := range c.Values {
			if strings.EqualFold(ctx.Method, v) {
				return true
			}
		}
		return false
	case rulespec.ConditionTypeHeader:
		v, ok := ctx.Headers[c.Key]
		if !ok {
			return false
		}
		switch c.Op {
		case rulespec.ConditionOpEquals:
			return v == c.Value
		case rulespec.ConditionOpContains:
			return strings.Contains(v, c.Value)
		case rulespec.ConditionOpRegex:
			return matchRegex(v, c.Value)
		default:
			return true
		}
	case rulespec.ConditionTypeQuery:
		v, ok := ctx.Query[c.Key]
		if !ok {
			return false
		}
		switch c.Op {
		case rulespec.ConditionOpEquals:
			return v == c.Value
		case rulespec.ConditionOpContains:
			return strings.Contains(v, c.Value)
		case rulespec.ConditionOpRegex:
			return matchRegex(v, c.Value)
		default:
			return true
		}
	case rulespec.ConditionTypeCookie:
		v, ok := ctx.Cookies[c.Key]
		if !ok {
			return false
		}
		switch c.Op {
		case rulespec.ConditionOpEquals:
			return v == c.Value
		case rulespec.ConditionOpContains:
			return strings.Contains(v, c.Value)
		case rulespec.ConditionOpRegex:
			return matchRegex(v, c.Value)
		default:
			return true
		}
	case rulespec.ConditionTypeText:
		if ctx.Body == "" {
			return false
		}
		switch c.Op {
		case rulespec.ConditionOpEquals:
			return ctx.Body == c.Value
		case rulespec.ConditionOpContains:
			return strings.Contains(ctx.Body, c.Value)
		case rulespec.ConditionOpRegex:
			return matchRegex(ctx.Body, c.Value)
		default:
			return true
		}
	case rulespec.ConditionTypeMIME:
		s := strings.ToLower(ctx.ContentType)
		p := strings.ToLower(c.Pattern)
		switch c.Mode {
		case rulespec.ConditionModeExact:
			return s == p
		case rulespec.ConditionModePrefix:
			return strings.HasPrefix(s, p)
		default:
			return strings.HasPrefix(s, p)
		}
	case rulespec.ConditionTypeSize:
		var n int64
		if ctx.Body != "" {
			n = int64(len(ctx.Body))
		} else {
			if v, ok := ctx.Headers["content-length"]; ok {
				if x, err := parseInt64(v); err == nil {
					n = x
				} else {
					return false
				}
			} else {
				return false
			}
		}
		switch c.Op {
		case rulespec.ConditionOpLT:
			x, err := parseInt64(c.Value)
			if err != nil {
				return false
			}
			return n < x
		case rulespec.ConditionOpLTE:
			x, err := parseInt64(c.Value)
			if err != nil {
				return false
			}
			return n <= x
		case rulespec.ConditionOpGT:
			x, err := parseInt64(c.Value)
			if err != nil {
				return false
			}
			return n > x
		case rulespec.ConditionOpGTE:
			x, err := parseInt64(c.Value)
			if err != nil {
				return false
			}
			return n >= x
		case rulespec.ConditionOpEquals:
			x, err := parseInt64(c.Value)
			if err != nil {
				return false
			}
			return n == x
		case rulespec.ConditionOpBetween:
			parts := strings.SplitN(c.Value, ":", 2)
			if len(parts) != 2 {
				return false
			}
			a, err1 := parseInt64(strings.TrimSpace(parts[0]))
			b, err2 := parseInt64(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil {
				return false
			}
			if a > b {
				a, b = b, a
			}
			return n >= a && n <= b
		default:
			return true
		}
	case rulespec.ConditionTypeProbability:
		p := 0.0
		if c.Value != "" {
			if f, err := strconv.ParseFloat(c.Value, 64); err == nil {
				if f < 0 {
					f = 0
				}
				if f > 1 {
					f = 1
				}
				p = f
			}
		}
		return rand.Float64() < p
	case rulespec.ConditionTypeTimeWindow:
		// Value 格式: "HH:MM-HH:MM"
		parts := strings.SplitN(c.Value, "-", 2)
		if len(parts) != 2 {
			return false
		}
		s1 := strings.TrimSpace(parts[0])
		s2 := strings.TrimSpace(parts[1])
		toMin := func(s string) (int, bool) {
			t := strings.SplitN(s, ":", 2)
			if len(t) != 2 {
				return 0, false
			}
			h, err1 := strconv.Atoi(t[0])
			m, err2 := strconv.Atoi(t[1])
			if err1 != nil || err2 != nil {
				return 0, false
			}
			if h < 0 || h > 23 || m < 0 || m > 59 {
				return 0, false
			}
			return h*60 + m, true
		}
		a, ok1 := toMin(s1)
		b, ok2 := toMin(s2)
		if !ok1 || !ok2 {
			return false
		}
		now := time.Now()
		cur := now.Hour()*60 + now.Minute()
		if a <= b {
			return cur >= a && cur <= b
		}
		// 跨午夜窗口
		return cur >= a || cur <= b
	case rulespec.ConditionTypeJSONPointer:
		if ctx.Body == "" {
			return false
		}
		val, ok := jsonPointer(ctx.Body, c.Pointer)
		if !ok {
			return false
		}
		s := val
		switch c.Op {
		case rulespec.ConditionOpEquals:
			return s == c.Value
		case rulespec.ConditionOpContains:
			return strings.Contains(s, c.Value)
		case rulespec.ConditionOpRegex:
			return matchRegex(s, c.Value)
		default:
			return true
		}
	default:
		return false
	}
}

// parseInt64 将数字字符串解析为int64
func parseInt64(s string) (int64, error) {
	var n int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, strconv.ErrSyntax
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// jsonPointer 依据JSON Pointer从Body中读取值为字符串
func jsonPointer(body, ptr string) (string, bool) {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return "", false
	}
	if ptr == "" || ptr[0] != '/' {
		return "", false
	}
	cur := v
	tokens := splitPtr(ptr)
	for _, t := range tokens {
		switch c := cur.(type) {
		case map[string]any:
			tv, ok := c[t]
			if !ok {
				return "", false
			}
			cur = tv
		case []any:
			idx, ok := toIndex(t)
			if !ok || idx < 0 || idx >= len(c) {
				return "", false
			}
			cur = c[idx]
		default:
			return "", false
		}
	}
	switch x := cur.(type) {
	case string:
		return x, true
	case float64:
		return formatFloat(x), true
	case bool:
		if x {
			return "true", true
		} else {
			return "false", true
		}
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// splitPtr 将JSON Pointer切分为令牌序列
func splitPtr(p string) []string {
	var out []string
	i := 1
	for i < len(p) {
		j := i
		for j < len(p) && p[j] != '/' {
			j++
		}
		tok := p[i:j]
		tok = strings.ReplaceAll(tok, "~1", "/")
		tok = strings.ReplaceAll(tok, "~0", "~")
		out = append(out, tok)
		i = j + 1
	}
	return out
}

// toIndex 将字符串转换为数组索引
func toIndex(s string) (int, bool) {
	n := 0
	if len(s) == 0 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// formatFloat 将浮点数以尽量紧凑的十进制字符串表示
func formatFloat(f float64) string {
	if float64(int64(f)) == f {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Stats 返回规则引擎的命中统计信息
func (e *Engine) Stats() model.EngineStats {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := make(map[model.RuleID]int64, len(e.byRule))
	for k, v := range e.byRule {
		m[k] = v
	}
	return model.EngineStats{Total: e.total, Matched: e.matched, ByRule: m}
}

// matchRegex 使用缓存的正则进行匹配
func matchRegex(s, pattern string) bool {
	re, err := regexCache.Get(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

// glob 简易通配符匹配，仅支持前后缀*
func glob(s, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(s, strings.TrimPrefix(pattern, "*")) {
		return true
	}
	if strings.HasSuffix(pattern, "*") && strings.HasPrefix(s, strings.TrimSuffix(pattern, "*")) {
		return true
	}
	return s == pattern
}
