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
	"time"

	"go.uber.org/zap"
)

// DefaultErrorSuppressionSummaryInterval bounds how often a periodic summary is
// emitted while identical errors keep repeating, so an ongoing failure stays
// visible at normal log levels without flooding the log.
const DefaultErrorSuppressionSummaryInterval = 60 * time.Second

// DefaultErrorDedupResetInterval is how long LogErrorDedup waits without being
// called before treating the next call as a fresh error cycle. When a caller
// stops reporting an error (because it cleared), the following call after this
// interval restarts deduplication automatically, so callers never need to call
// Reset by hand.
const DefaultErrorDedupResetInterval = 5 * time.Minute

// DedupLogger is a *zap.SugaredLogger that additionally deduplicates repeating
// error logs from a hot loop (such as an FSM reconcile loop). It embeds the
// SugaredLogger, so all standard logging methods are available directly, plus
// LogErrorDedup for the deduplicated path.
//
// LogErrorDedup logs the first occurrence and first repeat at Error, demotes
// further repeats to Debug, and emits a periodic Error summary of the
// suppressed count. A changed message ends the cycle with a final summary. When
// the error clears the caller simply stops calling LogErrorDedup; the next call
// after DefaultErrorDedupResetInterval starts a fresh cycle automatically.
//
// It is not safe for concurrent use; callers that share it across goroutines
// must serialize access.
type DedupLogger struct {
	*zap.SugaredLogger

	// dedupSugar is SugaredLogger with one extra caller-skip frame, used for the
	// LogErrorDedup path so the reported caller is the code that called
	// LogErrorDedup rather than dedup.go itself. Promoted methods on the embedded
	// SugaredLogger keep the normal caller depth.
	dedupSugar *zap.SugaredLogger

	// lastSuppressionSummary is the time the most recent suppression summary was
	// emitted, used to throttle the periodic summary.
	lastSuppressionSummary time.Time

	// lastLogTime is the time LogErrorDedup was last called, used to restart the
	// cycle after the error has been quiet for resetInterval.
	lastLogTime time.Time

	// lastLoggedErrorMsg is the message of the currently deduplicated error.
	lastLoggedErrorMsg string

	// summaryInterval bounds how often the periodic summary is emitted.
	summaryInterval time.Duration

	// resetInterval is the quiet period after which the next LogErrorDedup call
	// is treated as a fresh error cycle.
	resetInterval time.Duration

	// suppressedErrorCount counts repeats demoted to Debug since the last
	// summary. Reported and reset by the periodic and final summaries.
	suppressedErrorCount uint64

	// errorSuppressionAnnounced records whether the "repeats suppressed" notice
	// was already logged for the current message, so it is emitted exactly once.
	errorSuppressionAnnounced bool
}

// NewDedupLogger returns a DedupLogger writing to l with the default summary and
// reset intervals.
func NewDedupLogger(l *zap.SugaredLogger) *DedupLogger {
	return &DedupLogger{
		SugaredLogger:   l,
		dedupSugar:      l.WithOptions(zap.AddCallerSkip(1)),
		summaryInterval: DefaultErrorSuppressionSummaryInterval,
		resetInterval:   DefaultErrorDedupResetInterval,
	}
}

// LogErrorDedup logs an error without spamming: first occurrence at Error, first
// repeat at Error with a suppression note, further repeats at Debug plus a
// periodic Error summary of the suppressed count. A changed message resets the
// cycle and emits a final Error summary of what was suppressed. When the caller
// stops reporting for resetInterval (because the error cleared), the next call
// starts a fresh cycle automatically.
func (d *DedupLogger) LogErrorDedup(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	now := time.Now()

	// If the error has been quiet long enough, it is considered cleared: emit a
	// final summary and start over.
	if !d.lastLogTime.IsZero() && now.Sub(d.lastLogTime) >= d.resetInterval {
		d.Reset()
	}
	d.lastLogTime = now

	switch {
	case msg != d.lastLoggedErrorMsg:
		d.emitSuppressionSummary()
		d.dedupSugar.Errorf("%s", msg)
		d.lastLoggedErrorMsg = msg
		d.errorSuppressionAnnounced = false
		d.suppressedErrorCount = 0
		d.lastSuppressionSummary = now
	case !d.errorSuppressionAnnounced:
		d.dedupSugar.Errorf("%s (further repeats suppressed to debug until it changes or clears)", msg)
		d.errorSuppressionAnnounced = true
		d.lastSuppressionSummary = now
	default:
		d.dedupSugar.Debugf("%s", msg)
		d.suppressedErrorCount++
		if now.Sub(d.lastSuppressionSummary) >= d.summaryInterval {
			d.dedupSugar.Errorf("repeated error suppressed %d times in the last %s (still failing): %s",
				d.suppressedErrorCount, now.Sub(d.lastSuppressionSummary).Round(time.Second), msg)
			d.suppressedErrorCount = 0
			d.lastSuppressionSummary = now
		}
	}
}

// emitSuppressionSummary logs a final Error summary when a suppressed error
// changes or clears, so the scale of an ongoing failure is not hidden. It is a
// no-op when no repeats were suppressed since the last summary.
func (d *DedupLogger) emitSuppressionSummary() {
	if d.suppressedErrorCount == 0 {
		return
	}
	d.dedupSugar.Errorf("previous error cleared or changed after %d further suppressed repeats: %s",
		d.suppressedErrorCount, d.lastLoggedErrorMsg)
	d.suppressedErrorCount = 0
}

// Reset clears the dedup cycle, emitting a final summary of any suppressed
// repeats. Callers with an explicit success signal (such as a reconcile loop
// that just succeeded) may call it directly; otherwise the reset interval
// handles it automatically.
func (d *DedupLogger) Reset() {
	d.emitSuppressionSummary()
	d.lastLoggedErrorMsg = ""
	d.errorSuppressionAnnounced = false
}
