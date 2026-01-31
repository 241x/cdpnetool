package gui

import (
	"encoding/json"
	"errors"
	"strings"

	"cdpnetool/pkg/domain"
)

// 错误码常量
const (
	CodeSessionNotFound     = "SESSION_NOT_FOUND"
	CodeSessionStartFailed  = "SESSION_START_FAILED"
	CodeNoTargetAttached    = "NO_TARGET_ATTACHED"
	CodeTargetNotFound      = "TARGET_NOT_FOUND"
	CodeDevToolsUnreachable = "DEVTOOLS_UNREACHABLE"
	CodeNetworkError        = "NETWORK_ERROR"
	CodeInvalidConfig       = "INVALID_CONFIG"
	CodeConfigNotFound      = "CONFIG_NOT_FOUND"
	CodeBrowserNotRunning   = "BROWSER_NOT_RUNNING"
	CodeBrowserStartFailed  = "BROWSER_START_FAILED"
	CodeDatabaseError       = "DATABASE_ERROR"
	CodeUnknown             = "UNKNOWN_ERROR"
)

// 错误映射表
var errorMappings = map[error]struct {
	Code    string
	Message string
}{
	domain.ErrSessionNotFound: {
		Code:    CodeSessionNotFound,
		Message: "会话不存在，请先启动会话",
	},
	domain.ErrDevToolsUnreachable: {
		Code:    CodeDevToolsUnreachable,
		Message: "无法连接到浏览器，请检查 DevTools 地址是否正确",
	},
	domain.ErrNoTargetAttached: {
		Code:    CodeNoTargetAttached,
		Message: "请先在 Targets 标签页附加至少一个目标",
	},
	domain.ErrBrowserNotRunning: {
		Code:    CodeBrowserNotRunning,
		Message: "浏览器未运行",
	},
	domain.ErrBrowserStartFailed: {
		Code:    CodeBrowserStartFailed,
		Message: "浏览器启动失败，请检查系统是否安装了 Chrome 或 Edge",
	},
	domain.ErrInvalidConfig: {
		Code:    CodeInvalidConfig,
		Message: "配置格式错误，请检查 JSON 格式是否正确",
	},
	domain.ErrConfigNotFound: {
		Code:    CodeConfigNotFound,
		Message: "配置不存在",
	},
	domain.ErrDatabaseNotInitialized: {
		Code:    CodeDatabaseError,
		Message: "数据库未初始化，请重启应用",
	},
}

// translateError 将领域错误转换为用户友好的错误码和消息
func (a *App) translateError(err error) (code, message string) {
	if err == nil {
		return "", ""
	}

	// 尝试匹配已知的领域错误
	for domainErr, info := range errorMappings {
		if errors.Is(err, domainErr) {
			a.log.Err(err, "业务错误", "code", info.Code)
			return info.Code, info.Message
		}
	}

	// 处理网络相关错误
	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "websocket: bad handshake") {
		a.log.Err(err, "网络连接错误")
		return CodeNetworkError, "无法连接到浏览器，请确保浏览器已开启 DevTools 远程调试"
	}

	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") {
		a.log.Err(err, "网络超时")
		return CodeNetworkError, "网络请求超时，请稍后重试"
	}

	// 处理 JSON 解析错误
	var jsonErr *json.SyntaxError
	if errors.As(err, &jsonErr) {
		a.log.Err(err, "JSON解析错误")
		return CodeInvalidConfig, "配置格式错误，请检查 JSON 语法"
	}

	// 未知错误
	a.log.Err(err, "未知错误")
	return CodeUnknown, "操作失败，请查看日志了解详情"
}
