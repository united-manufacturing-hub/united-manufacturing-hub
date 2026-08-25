// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// DefaultThrottleInterval bounds how often a throttled warning is emitted per key.
const DefaultThrottleInterval = 60 * time.Second
const DefaultEscalateCounts = 100

// throttleEntry tracks the last emission and the suppressed count for one key.
type throttleEntry struct {
	lastLogged time.Time
	suppressed uint64
}

// throttledLogger is a process-wide throttle keyed by an explicit string: each
// key logs once, then suppresses repeats for interval and folds the suppressed
// count into the next emission. Safe for concurrent use.
type throttledLogger struct {
	logger   *zap.SugaredLogger
	entries  map[string]*throttleEntry
	interval time.Duration
	mu       sync.Mutex
}

// reconcileLogger is the process-wide throttle for transient reconcile-loop
// conditions that would otherwise log once per instance per tick.
var reconcileLogger = &throttledLogger{
	entries:  make(map[string]*throttleEntry),
	interval: DefaultThrottleInterval,
}

// printfFor returns the SugaredLogger's printf-style method for the given level,
// defaulting to Warnf for unknown levels.
func (w *throttledLogger) printfFor(level zapcore.Level) func(string, ...any) {
	switch level {
	case zapcore.ErrorLevel:
		return w.logger.Errorf
	case zapcore.InfoLevel:
		return w.logger.Infof
	case zapcore.DebugLevel:
		return w.logger.Debugf
	default:
		return w.logger.Warnf
	}
}

func (w *throttledLogger) log(key string, warnMsg string, level zapcore.Level, escalate bool, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.logger == nil {
		w.logger = For("reconcile").WithOptions(zap.AddCallerSkip(2))
	}

	if len(args) > 0 {
		warnMsg = fmt.Sprintf(warnMsg, args...)
	}

	now := time.Now()

	entry := w.entries[key]
	if entry == nil {
		entry = &throttleEntry{}
		w.entries[key] = entry
	}

	// First occurrence, or a full interval has elapsed: emit now, folding in how
	// many occurrences were suppressed since the last emission.
	if entry.lastLogged.IsZero() || now.Sub(entry.lastLogged) >= w.interval {
		switch {
		case entry.suppressed == 0:
			w.printfFor(level)("%s", warnMsg)
		case escalate && entry.suppressed > DefaultEscalateCounts:
			w.logger.Errorf("%s (%d further occurrences across all instances in the last %s)",
				warnMsg, entry.suppressed, now.Sub(entry.lastLogged).Round(time.Second))
		default:
			w.printfFor(level)("%s (%d further occurrences across all instances in the last %s)",
				warnMsg, entry.suppressed, now.Sub(entry.lastLogged).Round(time.Second))
		}

		entry.lastLogged = now
		entry.suppressed = 0

		return
	}

	entry.suppressed++

	// Second occurrence overall: announce throttling immediately rather than
	// waiting for the next interval boundary. Emitted once per throttle window.
	if entry.suppressed == 1 {
		w.printfFor(level)("%s (suppressing further occurrences for %s)", warnMsg, w.interval)
	}
}

// ResetThrottleLoggerForTest points the throttled logger at l and forgets every
// key. Without it the destination is resolved once, on the first ThrottledX call
// anywhere in the process, and cached - so a test in another package can never
// observe what these helpers emit, and its keys stay throttled across specs.
func ResetThrottleLoggerForTest(l *zap.SugaredLogger) {
	reconcileLogger.mu.Lock()
	defer reconcileLogger.mu.Unlock()

	reconcileLogger.logger = l
	reconcileLogger.entries = make(map[string]*throttleEntry)
}

// ThrottledError logs a transient shared-code condition at Error, throttled per
// key. Use when no single caller owns the error; for an owned error stream with
// a success signal, use DedupLogger instead.
func ThrottledError(key, warnMsg string, debugArgs ...any) {
	reconcileLogger.log(key, warnMsg, zapcore.ErrorLevel, false, debugArgs...)
}

// ThrottledWarn logs a transient shared-code condition at Warn, throttled per key.
func ThrottledWarn(key, warnMsg string, debugArgs ...any) {
	reconcileLogger.log(key, warnMsg, zapcore.WarnLevel, false, debugArgs...)
}

// ThrottledInfo logs a transient shared-code condition at Info, throttled per key.
func ThrottledInfo(key, warnMsg string, debugArgs ...any) {
	reconcileLogger.log(key, warnMsg, zapcore.InfoLevel, false, debugArgs...)
}

// ThrottledDebug logs a transient shared-code condition at Debug, throttled per key.
func ThrottledDebug(key, warnMsg string, debugArgs ...any) {
	reconcileLogger.log(key, warnMsg, zapcore.DebugLevel, false, debugArgs...)
}

// EscalatingWarn logs at Warn, throttled per key, and escalates to Error once the
// suppressed count exceeds DefaultEscalateCounts — a condition that keeps firing
// gets louder rather than staying hidden.
func EscalatingWarn(key, warnMsg string, debugArgs ...any) {
	reconcileLogger.log(key, warnMsg, zapcore.WarnLevel, true, debugArgs...)
}

// EscalatingInfo logs at Info, throttled per key, escalating to Error once the
// suppressed count exceeds DefaultEscalateCounts.
func EscalatingInfo(key, warnMsg string, debugArgs ...any) {
	reconcileLogger.log(key, warnMsg, zapcore.InfoLevel, true, debugArgs...)
}

// EscalatingDebug logs at Debug, throttled per key, escalating to Error once the
// suppressed count exceeds DefaultEscalateCounts.
func EscalatingDebug(key, warnMsg string, debugArgs ...any) {
	reconcileLogger.log(key, warnMsg, zapcore.DebugLevel, true, debugArgs...)
}
