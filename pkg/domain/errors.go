package domain

import "errors"

// 会话相关错误
var (
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionAlreadyStop = errors.New("session already stopped")
	ErrSessionStartFailed = errors.New("session start failed")
)

// 目标相关错误
var (
	ErrNoTargetAttached = errors.New("no target attached")
	ErrTargetNotFound   = errors.New("target not found")
)

// 连接相关错误
var (
	ErrDevToolsUnreachable = errors.New("devtools unreachable")
	ErrNetworkTimeout      = errors.New("network timeout")
	ErrConnectionRefused   = errors.New("connection refused")
)

// 配置相关错误
var (
	ErrInvalidConfig  = errors.New("invalid config")
	ErrConfigNotFound = errors.New("config not found")
)

// 浏览器相关错误
var (
	ErrBrowserNotRunning  = errors.New("browser not running")
	ErrBrowserStartFailed = errors.New("browser start failed")
)

// 数据库相关错误
var (
	ErrDatabaseNotInitialized = errors.New("database not initialized")
	ErrRecordNotFound         = errors.New("record not found")
)
