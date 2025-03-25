package sentry

import (
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
)

func getDash(inp string) string {
	// Generate enough = to fill the length of inp
	return strings.Repeat("=", len(inp))
}

// reportFatal sends a fatal error to Sentry, including a stack trace and a message
// Afterwards it will report the error to the logger and panic
func reportFatal(err error, log *zap.SugaredLogger) {
	event := createSentryEvent(sentry.LevelFatal, err)
	sendSentryEvent(event)

	sentry.Flush(time.Second * 5)
	log.Error("The UMH-Core has encountered a fatal error and will now terminate. Please contact our customer support.")
	log.Errorf("Error: %s", err)
	log.Errorf("Stack trace: %s", string(debug.Stack()))
	log.Panic("Fatal error")
}

var errorLastSent time.Time = time.Now().Add(-time.Hour * 24)
var errorLastSentMutex sync.Mutex = sync.Mutex{}

// reportError sends an error to Sentry, including a stack trace and a message
// Afterwards it will report the error to the logger
func reportError(err error, log *zap.SugaredLogger) {
	errorLastSentMutex.Lock()
	defer errorLastSentMutex.Unlock()

	if time.Since(errorLastSent) < time.Hour*2 {
		return
	}

	event := createSentryEvent(sentry.LevelError, err)
	sendSentryEvent(event)
	errorLastSent = time.Now()
	log.Error(err)
}

var warningLastSent time.Time = time.Now().Add(-time.Hour * 24)
var warningLastSentMutex sync.Mutex = sync.Mutex{}

// reportWarning sends a warning to Sentry, including a stack trace and a message
// Afterwards it will report the warning to the logger
func reportWarning(err error, log *zap.SugaredLogger) {
	warningLastSentMutex.Lock()
	defer warningLastSentMutex.Unlock()

	if time.Since(warningLastSent) < time.Hour*2 {
		return
	}

	event := createSentryEvent(sentry.LevelWarning, err)
	sendSentryEvent(event)
	warningLastSent = time.Now()
	log.Warn(err)
}
