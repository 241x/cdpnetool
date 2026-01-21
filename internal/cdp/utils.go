package cdp

import (
	"fmt"
	"strings"
)

// ParseCookie 解析Cookie头为键值对映射
func ParseCookie(s string) map[string]string {
	out := make(map[string]string)
	parts := strings.Split(s, ";")
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

// shouldGetBody 判断是否应该获取Body内容（基于Content-Type和大小）
func shouldGetBody(ctype string, clen int64, thr int64) bool {
	if thr <= 0 {
		thr = 4 * 1024 * 1024
	}
	if clen > 0 && clen > thr {
		return false
	}
	lc := strings.ToLower(ctype)
	if strings.HasPrefix(lc, "text/") {
		return true
	}
	if strings.HasPrefix(lc, "application/json") {
		return true
	}
	return false
}

// parseInt64 简单的正整数解析
func parseInt64(s string) (int64, error) {
	var n int64
	var mul int64 = 1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid")
		}
		n = n*10 + int64(c-'0')
	}
	return n * mul, nil
}
