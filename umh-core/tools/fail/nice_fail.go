package fail

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/hwid"

	"github.com/DataDog/gostackparse"
	"go.uber.org/zap"
)

/// TAG

type SentryTag string

const (
	UmhEnabled         SentryTag = "umh.enabled"
	HasKafkaConnection SentryTag = "kafka.connected"
)

var tagMap = make(map[SentryTag]string)
var tagMutex = sync.RWMutex{}

func SetTag(tag SentryTag, value string) {
	tagMutex.Lock()
	defer tagMutex.Unlock()
	tagMap[tag] = value
}

/// WATCHDOG

type WatchdogState struct {
	LastStatus    string
	LastHeartBeat int64 // Since unix epoch in seconds
	WarningCount  uint32
}

var watchdogStates = make(map[string]*WatchdogState)
var watchdogMutex = sync.RWMutex{}

func WatchdogReport(name string, status string, lastHeartbeat int64, warningCount uint32) {
	watchdogMutex.Lock()
	defer watchdogMutex.Unlock()
	state, ok := watchdogStates[name]
	if !ok {
		state = &WatchdogState{}
	}
	state.LastStatus = status
	state.LastHeartBeat = lastHeartbeat
	state.WarningCount = warningCount
	watchdogStates[name] = state
}

/// SENTRY

// Helper function to create a new Sentry event
func createSentryEvent(level sentry.Level, err error, amount int, additionalAttachments []sentry.Attachment, additionalContexts map[string]sentry.Context) *sentry.Event {
	event := sentry.NewEvent()
	event.Level = level
	event.Message = err.Error()
	event.SetException(err, 1)

	// Nicer error string in sentry
	err0 := strings.Split(err.Error(), ":")[0]
	if len(event.Exception) > 0 {
		event.Exception[0].Type = err0
	}
	if amount > -1 {
		event.Extra["amount"] = amount
	}

	event.Tags["hwid"] = hwid.GenerateHWID()
	// Add tags from map
	tagMutex.RLock()
	for k, v := range tagMap {
		event.Tags[string(k)] = v
	}
	tagMutex.RUnlock()

	// Add watchdog states as Extra
	watchdogMutex.RLock()
	for k, v := range watchdogStates {
		vCopy := *v
		event.Extra[k] = vCopy
	}
	watchdogMutex.RUnlock()

	// Add additional attachment if provided
	if additionalAttachments != nil {
		for _, attachment := range additionalAttachments {
			event.Attachments = append(event.Attachments, &attachment)
		}
	}

	// Capture all goroutines and convert them to Sentry threads
	if level == sentry.LevelFatal {
		threads, stacktrace := captureGoroutinesAsThreads()
		event.Threads = threads
		event.Attachments = append(event.Attachments, &sentry.Attachment{
			Filename:    "stacktrace.txt",
			ContentType: "text/plain",
			Payload:     stacktrace,
		})
	}

	// Add additional contexts
	for k, v := range additionalContexts {
		// Don't overwrite default context (Device, Runtime, Operating System)
		if k == "Device" || k == "Runtime" || k == "Operating System" {
			continue
		}
		event.Contexts[k] = v
	}

	return event
}

// Helper function to send an event to Sentry
func sendSentryEvent(event *sentry.Event) {
	localHub := sentry.CurrentHub().Clone()
	localHub.CaptureEvent(event)
}

var logOnceMutex = sync.Mutex{}

// General function for logging and sending batched messages
func logAndSendBatched(err error, amountMap *expiremap.ExpireMap[string, int], lastSendMap *expiremap.ExpireMap[string, time.Time], sendFunc func(err error, amount int, attachment []sentry.Attachment, additionalContexts map[string]sentry.Context), attachment []sentry.Attachment, additionalContexts map[string]sentry.Context) {
	logOnceMutex.Lock()
	defer logOnceMutex.Unlock()

	errStr := err.Error()
	store, _ := amountMap.LoadOrStore(errStr, 0)
	*store = (*store) + 1
	amountMap.Set(errStr, *store)

	lastErr, _ := lastSendMap.LoadOrStore(errStr, time.Unix(0, 0))
	if time.Since(*lastErr) < 1*time.Hour {
		return
	}
	sendFunc(err, *store, attachment, additionalContexts)
	lastSendMap.Set(errStr, time.Now())
	amountMap.Set(errStr, 0)
}

func sendError(err error, amount int, attachment []sentry.Attachment, additionalContexts map[string]sentry.Context) {
	event := createSentryEvent(sentry.LevelError, err, amount, attachment, additionalContexts)
	sendSentryEvent(event)
}

func sendWarning(err error, amount int, attachment []sentry.Attachment, additionalContexts map[string]sentry.Context) {
	event := createSentryEvent(sentry.LevelWarning, err, amount, attachment, additionalContexts)
	sendSentryEvent(event)
}

// Fatal error handler
func Fatal(err string) {
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

	event := createSentryEvent(sentry.LevelFatal, errors.New(err), -1, nil, nil)
	sendSentryEvent(event)

	sentry.Flush(time.Second * 5)
	panic(err)
}

func Fatalf(template string, args ...interface{}) {
	Fatal(fmt.Sprintf(template, args...))
}

var errorAmountMap = expiremap.NewEx[string, int](time.Minute, 2*time.Hour)
var errorLastSendMap = expiremap.NewEx[string, time.Time](time.Minute, 2*time.Hour)

// ErrorNow logs an error message to sentry and zap
// This is a costly operation, please consider using ErrorBatched instead
func ErrorNow(err error) {
	sendError(err, -1, nil, nil)
}

// ErrorNowf logs an error message to sentry and zap
// This is a costly operation, please consider using ErrorBatchedf instead
func ErrorNowf(template string, args ...interface{}) {
	ErrorNow(fmt.Errorf(template, args...))
}

// ErrorBatched logs an error message to sentry and zap
// The first invocation with a specific error message will send the warning,
// afterward it will accumulate the number of warnings and send them in a batch every 1 hour
// Please use this function over the normal ErrorNow
func ErrorBatched(err error) {
	zap.S().Error(err)
	logAndSendBatched(err, errorAmountMap, errorLastSendMap, sendError, nil, nil)
}

// ErrorBatchedf logs an error message to sentry and zap
// It accumulates the number of warnings and sends them in a batch every 1 hour
// Please use this function over the normal ErrorNowf
func ErrorBatchedf(template string, args ...interface{}) {
	ErrorBatched(fmt.Errorf(template, args...))
}

func ErrorBatchedfWithAttachment(template string, attachment []sentry.Attachment, args ...interface{}) {
	logAndSendBatched(fmt.Errorf(template, args...), errorAmountMap, errorLastSendMap, sendError, attachment, nil)
}

// Add the new function with additional context support
func ErrorBatchedfWithAttachmentAndContext(template string, attachment []sentry.Attachment, additionalContexts map[string]sentry.Context, args ...interface{}) {
	logAndSendBatched(fmt.Errorf(template, args...), errorAmountMap, errorLastSendMap, sendError, attachment, additionalContexts)
}

var warnAmountMap = expiremap.NewEx[string, int](time.Minute, 2*time.Hour)
var warnLastSendMap = expiremap.NewEx[string, time.Time](time.Minute, 2*time.Hour)

// WarnNow logs a warning message to sentry and zap
// This is a costly operation, please consider using WarnBatched instead
func WarnNow(err error) {
	sendWarning(err, -1, nil, nil)
}

// WarnNowf logs a warning message to sentry and zap
// This is a costly operation, please consider using WarnBatchedf instead
func WarnNowf(template string, args ...interface{}) {
	WarnNow(fmt.Errorf(template, args...))
}

// WarnBatched logs a warning message to sentry and zap only once
// The first invocation with a specific error message will send the warning,
// afterward it will accumulate the number of warnings and send them in a batch every 1 hour
// Please use this function over the normal WarnNow
func WarnBatched(err error) {
	zap.S().Warn(err)
	logAndSendBatched(err, warnAmountMap, warnLastSendMap, sendWarning, nil, nil)
}

// WarnBatchedf logs a warning message to sentry and zap only once
// The first invocation with a specific error message will send the warning,
// afterward it will accumulate the number of warnings and send them in a batch every 1 hour
// Please use this function over the normal WarnNowf
func WarnBatchedf(template string, args ...interface{}) {
	WarnBatched(fmt.Errorf(template, args...))
}

func WarnBatchedfWithAttachment(template string, attachment []sentry.Attachment, args ...interface{}) {
	logAndSendBatched(fmt.Errorf(template, args...), warnAmountMap, warnLastSendMap, sendWarning, attachment, nil)
}

// Add the new function with additional context support for warnings
func WarnBatchedfWithAttachmentAndContext(template string, attachment []sentry.Attachment, additionalContexts map[string]sentry.Context, args ...interface{}) {
	logAndSendBatched(fmt.Errorf(template, args...), warnAmountMap, warnLastSendMap, sendWarning, attachment, additionalContexts)
}

func getDash(inp string) string {
	// Generate enough = to fill the length of inp
	return strings.Repeat("=", len(inp))
}

// captureGoroutinesAsThreads captures all current goroutines and converts them to Sentry threads.
func captureGoroutinesAsThreads() ([]sentry.Thread, []byte) {
	// Capture the current stack trace for all goroutines
	stack := EntireStack()

	// Parse the stack trace using gostackparse
	goroutines, err := gostackparse.Parse(bytes.NewReader(stack))
	if err != nil {
		// Handle error (optional: log or return an empty list of threads)
		fmt.Printf("Error parsing goroutines: %v\n", err)
		return nil, []byte("")
	}

	// Convert parsed goroutines to Sentry.Thread format
	var threads []sentry.Thread

	for _, g := range goroutines {
		thread := convertGoroutineToThread(g)
		threads = append(threads, thread)
	}

	// Return the list of Sentry threads and also the raw stacktrace string for additional logging or debugging
	return threads, stack
}

func EntireStack() []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}

// convertGoroutineToThread converts a parsed Goroutine to a Sentry Thread object
func convertGoroutineToThread(g *gostackparse.Goroutine) sentry.Thread {
	// Convert each Goroutine's stack frames to Sentry frames
	frames := convertFrames(g.Stack)

	// Create a Sentry stacktrace
	stacktrace := &sentry.Stacktrace{
		Frames: frames,
	}

	// Create a Sentry thread
	return sentry.Thread{
		ID:         fmt.Sprintf("%d", g.ID),
		Name:       fmt.Sprintf("Goroutine %d", g.ID),
		Stacktrace: stacktrace,
		Crashed:    false, // Adjust based on actual crash status if needed
		Current:    false, // You can refine this if you track the "current" thread
	}
}

// convertFrames converts a slice of gostackparse.Frame to a slice of sentry.Frame
func convertFrames(goroutineFrames []*gostackparse.Frame) []sentry.Frame {
	var frames []sentry.Frame
	for _, gf := range goroutineFrames {
		frame := sentry.Frame{
			Function: gf.Func,
			Filename: gf.File,
			Lineno:   gf.Line,
			AbsPath:  gf.File,                  // Optional: add absolute path if available
			InApp:    shouldMarkInApp(gf.Func), // Optional logic to mark app frames
		}
		frames = append(frames, frame)
	}
	return frames
}

// shouldMarkInApp marks whether the function belongs to the application (can adjust as needed)
func shouldMarkInApp(functionName string) bool {
	// Example: marking frames as "in-app" based on your package structure
	// You can customize this based on your project's function and module names
	return !strings.Contains(functionName, "runtime.") && !strings.Contains(functionName, "third_party.")
}
