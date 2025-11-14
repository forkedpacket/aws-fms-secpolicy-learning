package util

import (
	"log"
	"os"
)

type Logger struct {
	l *log.Logger
}

func NewLogger() *Logger {
	return &Logger{
		l: log.New(os.Stderr, "[fms-poc] ", log.LstdFlags|log.Lshortfile),
	}
}

func (l *Logger) Infof(format string, args ...any) {
	l.l.Printf("INFO: "+format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.l.Printf("WARN: "+format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.l.Printf("ERROR: "+format, args...)
}
