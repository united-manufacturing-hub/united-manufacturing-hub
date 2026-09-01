package logger

import (
	"fmt"
	"time"
)

type LoggerFunc func(prefix string, message string, args ...any)

func StdOutLogger(prefix string, message string, args ...any) {
	now := time.Now()
	// Format the timestamp as YYYY-MM-DD HH:MM:SS.sss
	timestamp := now.Format("2006-01-02 15:04:05.000")
	fmt.Printf("[%s] %s: %s\n", timestamp, prefix, fmt.Sprintf(message, args...))
}

func NoopLogger(prefix string, message string, args ...any) {
	// Do nothing
}
