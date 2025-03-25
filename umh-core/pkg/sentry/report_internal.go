package sentry

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
)

func getDash(inp string) string {
	// Generate enough = to fill the length of inp
	return strings.Repeat("=", len(inp))
}

// Fatal error handler
func reportFatal(err string) {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(getDash(err))
	sb.WriteString("\n")
	sb.WriteString(err)
	sb.WriteString("\n")
	sb.WriteString("Please contact our customer support")
	sb.WriteString("\n")
	sb.WriteString(getDash(err))
	sb.WriteString("\n")
	sb.WriteString("Stack trace:")
	sb.WriteString(string(debug.Stack()))
	sb.WriteString("\n")
	fmt.Println(sb.String())

	event := createSentryEvent(sentry.LevelFatal, errors.New(err))
	sendSentryEvent(event)

	sentry.Flush(time.Second * 5)
	panic(err)
}

var warningLastSent time.Time = time.Now().Add(-time.Hour * 24)
var warningLastSentMutex sync.Mutex = sync.Mutex{}

func reportWarning(err string) {
	warningLastSentMutex.Lock()
	defer warningLastSentMutex.Unlock()

	if time.Since(warningLastSent) < time.Hour*2 {
		return
	}

	event := createSentryEvent(sentry.LevelWarning, errors.New(err))
	sendSentryEvent(event)
	warningLastSent = time.Now()
}

var errorLastSent time.Time = time.Now().Add(-time.Hour * 24)
var errorLastSentMutex sync.Mutex = sync.Mutex{}

func reportError(err string) {
	errorLastSentMutex.Lock()
	defer errorLastSentMutex.Unlock()

	if time.Since(errorLastSent) < time.Hour*2 {
		return
	}

	event := createSentryEvent(sentry.LevelError, errors.New(err))
	sendSentryEvent(event)
	errorLastSent = time.Now()

}
