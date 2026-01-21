package protocol

import (
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
