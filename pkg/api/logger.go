package api

import (
	"log"

	ilog "cdpnetool/internal/log"
)

func SetLogger(l *log.Logger) { ilog.Set(l) }
