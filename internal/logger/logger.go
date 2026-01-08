package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// LogLevel 定义日志级别
type LogLevel int

const (
	// LogLevelDebug 调试级别
	LogLevelDebug LogLevel = iota

	// LogLevelInfo 信息级别
	LogLevelInfo

	// LogLevelWarn 警告级别
	LogLevelWarn

	// LogLevelError 错误级别
	LogLevelError

	// LogLevelNone 禁用日志
	LogLevelNone
)

// String 返回日志级别的字符串表示
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelNone:
		return "NONE"
	default:
		return "UNKNOWN"
	}
}

// Logger 定义日志接口
type Logger interface {
	// Debug 记录调试信息
	Debug(format string, args ...any)

	// Info 记录一般信息
	Info(format string, args ...any)

	// Warn 记录警告信息
	Warn(format string, args ...any)

	// Error 记录错误信息
	Error(format string, args ...any)

	// SetLevel 设置日志级别
	SetLevel(level LogLevel)
}

// DefaultLogger 默认日志实现
type DefaultLogger struct {
	level  LogLevel
	logger *log.Logger
}

// NewDefaultLogger 创建默认日志记录器
func NewDefaultLogger(level LogLevel, output io.Writer) *DefaultLogger {
	if output == nil {
		output = os.Stdout
	}

	return &DefaultLogger{
		level:  level,
		logger: log.New(output, "", 0), // 不使用标准库的前缀,我们自己格式化
	}
}

// Debug 记录调试信息
func (l *DefaultLogger) Debug(format string, args ...any) {
	if l.level <= LogLevelDebug {
		l.log(LogLevelDebug, format, args...)
	}
}

// Info 记录一般信息
func (l *DefaultLogger) Info(format string, args ...any) {
	if l.level <= LogLevelInfo {
		l.log(LogLevelInfo, format, args...)
	}
}

// Warn 记录警告信息
func (l *DefaultLogger) Warn(format string, args ...any) {
	if l.level <= LogLevelWarn {
		l.log(LogLevelWarn, format, args...)
	}
}

// Error 记录错误信息
func (l *DefaultLogger) Error(format string, args ...any) {
	if l.level <= LogLevelError {
		l.log(LogLevelError, format, args...)
	}
}

// SetLevel 设置日志级别
func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.level = level
}

// log 内部日志方法
func (l *DefaultLogger) log(level LogLevel, message string, args ...any) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	if len(args)%2 != 0 {
		args = append(args, "MISSING")
	}

	// 添加键值对
	var others string
	for i := 0; i < len(args); i += 2 {
		key := fmt.Sprintf("%v", args[i])
		value := args[i+1]
		others += fmt.Sprintf(" %s=%v", key, value)
	}

	l.logger.Printf("[%s] [%s] \"%s\" %s", timestamp, level.String(), message, others)
}

// NoopLogger 空日志实现,不输出任何日志
type NoopLogger struct{}

// NewNoopLogger 创建空日志记录器
func NewNoopLogger() *NoopLogger {
	return &NoopLogger{}
}

// Debug 不执行任何操作
func (l *NoopLogger) Debug(format string, args ...any) {}

// Info 不执行任何操作
func (l *NoopLogger) Info(format string, args ...any) {}

// Warn 不执行任何操作
func (l *NoopLogger) Warn(format string, args ...any) {}

// Error 不执行任何操作
func (l *NoopLogger) Error(format string, args ...any) {}

// SetLevel 不执行任何操作
func (l *NoopLogger) SetLevel(level LogLevel) {}
