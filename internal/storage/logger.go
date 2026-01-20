package storage

import (
	"context"
	"time"

	logger2 "cdpnetool/internal/logger"

	"gorm.io/gorm/logger"
)

// GormLogger 自定义GORM logger实现
type GormLogger struct {
	logger2.Logger
	LogLevel logger.LogLevel
}

// NewGormLogger 创建新的GormLogger实例
func NewGormLogger(l logger2.Logger) *GormLogger {
	return &GormLogger{
		Logger:   l,
		LogLevel: logger.Info, // 默认日志级别
	}
}

// LogMode 设置日志级别
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

// Info 打印info级别日志
func (l *GormLogger) Info(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Info {
		l.Logger.Info(msg, data...)
	}
}

// Warn 打印warn级别日志
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Warn {
		l.Logger.Warn(msg, data...)
	}
}

// Error 打印error级别日志
func (l *GormLogger) Error(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Error {
		l.Logger.Error(msg, data...)
	}
}

// Trace 打印SQL日志
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []any{
		"sql", sql,
		"rows", rows,
		"timeMs", float64(elapsed.Nanoseconds()) / 1e6,
	}

	switch {
	case err != nil && l.LogLevel >= logger.Error:
		l.Logger.Error("SQL执行错误", append(fields, "error", err)...)
	case elapsed > time.Second && l.LogLevel >= logger.Warn:
		l.Logger.Warn("慢SQL查询", append(fields, "threshold", "1s")...)
	case l.LogLevel == logger.Info:
		l.Logger.Debug("SQL执行", fields...)
	}
}
