package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 定义结构化日志接口
type Logger interface {
	Debug(msg string, fields ...any)
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
	Err(err error, msg string, fields ...any)
	With(fields ...any) Logger
}

// Options 日志配置选项
type Options struct {
	Level      string   // 日志级别: debug, info, warn, error
	Writers    []string // 输出目标: console, file
	Filename   string   // 日志文件名 (如果为空则自动生成)
	MaxSize    int      // 每个日志文件最大 MB
	MaxBackups int      // 保留的最大旧文件数
	MaxAge     int      // 保留的最大天数
	Compress   bool     // 是否压缩旧文件
}

type zeroLogger struct {
	logger zerolog.Logger
}

// New 创建一个新的结构化日志记录器
func New(opts Options) Logger {
	level := zerolog.DebugLevel
	switch opts.Level {
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	}

	var writers []io.Writer
	for _, w := range opts.Writers {
		switch w {
		case "console":
			writers = append(writers, zerolog.ConsoleWriter{
				Out:        os.Stderr,
				TimeFormat: "2006-01-02 15:04:05.000",
			})
		case "file":
			filename := opts.Filename
			if filename == "" {
				var err error
				filename, err = GetDefaultLogPath()
				if err != nil {
					fmt.Fprintf(os.Stderr, "无法获取默认日志路径: %v\n", err)
					continue
				}
			}

			// 确保日志目录存在
			dir := filepath.Dir(filename)
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "无法创建日志目录 %s: %v\n", dir, err)
				continue
			}

			lumberjackLogger := &lumberjack.Logger{
				Filename:   filename,
				MaxSize:    opts.MaxSize,
				MaxBackups: opts.MaxBackups,
				MaxAge:     opts.MaxAge,
				Compress:   opts.Compress,
				LocalTime:  true,
			}
			if lumberjackLogger.MaxSize <= 0 {
				lumberjackLogger.MaxSize = 10 // 默认 10MB
			}
			if lumberjackLogger.MaxBackups <= 0 {
				lumberjackLogger.MaxBackups = 5
			}
			if lumberjackLogger.MaxAge <= 0 {
				lumberjackLogger.MaxAge = 30
			}

			writers = append(writers, lumberjackLogger)
		}
	}

	if len(writers) == 0 {
		return NewNop()
	}

	multi := io.MultiWriter(writers...)
	l := zerolog.New(multi).
		With().
		Timestamp().
		CallerWithSkipFrameCount(zerolog.CallerSkipFrameCount + 1).
		Logger().
		Level(level)

	return &zeroLogger{logger: l}
}

// NewNop 返回一个不执行任何操作的日志记录器
func NewNop() Logger {
	return &zeroLogger{logger: zerolog.Nop()}
}

func (z *zeroLogger) Debug(msg string, fields ...any) {
	z.logger.Debug().Fields(fields).Msg(msg)
}

func (z *zeroLogger) Info(msg string, fields ...any) {
	z.logger.Info().Fields(fields).Msg(msg)
}

func (z *zeroLogger) Warn(msg string, fields ...any) {
	z.logger.Warn().Fields(fields).Msg(msg)
}

func (z *zeroLogger) Error(msg string, fields ...any) {
	z.logger.Error().Fields(fields).Msg(msg)
}

func (z *zeroLogger) Err(err error, msg string, fields ...any) {
	z.logger.Err(err).Fields(fields).Msg(msg)
}

func (z *zeroLogger) With(fields ...any) Logger {
	return &zeroLogger{logger: z.logger.With().Fields(fields).Logger()}
}

// GetDefaultLogPath 获取平台相关的默认日志路径
func GetDefaultLogPath() (string, error) {
	var baseDir string
	switch runtime.GOOS {
	case "windows":
		baseDir = os.Getenv("APPDATA")
		if baseDir == "" {
			baseDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(home, "Library", "Application Support")
	default:
		baseDir = os.Getenv("XDG_DATA_HOME")
		if baseDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			baseDir = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(baseDir, "cdpnetool", "logs", "app.log"), nil
}
