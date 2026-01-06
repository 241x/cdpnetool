package log

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
}

type StdLogger struct{ l *log.Logger }

func New(l *log.Logger) Logger { return &StdLogger{l: l} }

func (s *StdLogger) Debug(msg string, args ...any) { s.write("DEBUG", msg, args...) }
func (s *StdLogger) Info(msg string, args ...any)  { s.write("INFO", msg, args...) }
func (s *StdLogger) Warn(msg string, args ...any)  { s.write("WARN", msg, args...) }
func (s *StdLogger) Error(msg string, args ...any) { s.write("ERROR", msg, args...) }

func (s *StdLogger) write(level, msg string, args ...any) {
    m := map[string]any{
        "time":  time.Now().Local().Format("2006-01-02 15:04:05"),
        "level": level,
        "msg":   msg,
    }
    for i := 0; i+1 < len(args); i += 2 {
        k := args[i]
        v := args[i+1]
        ks, ok := k.(string)
        if !ok { ks = "arg" }
        m[ks] = v
    }
    b, _ := json.Marshal(m)
    s.l.Print(string(b))
}

var defaultLogger = log.New(os.Stdout, "", 0)

func Set(l *log.Logger) { defaultLogger = l }

func L() *log.Logger { return defaultLogger }
