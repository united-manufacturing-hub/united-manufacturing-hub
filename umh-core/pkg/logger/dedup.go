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
// emitted while identical errors keep repeating.
const DefaultErrorSuppressionSummaryInterval = 60 * time.Second

// DefaultErrorDedupResetInterval is how long LogErrorDedup waits without being
// called before treating the next call as a fresh error cycle.
const DefaultErrorDedupResetInterval = 5 * time.Minute

// DedupLogger is a *zap.SugaredLogger that also deduplicates repeating error
// logs from a hot loop (such as an FSM reconcile loop)
//
// It is not safe for concurrent use.
type DedupLogger struct {
	*zap.SugaredLogger

	// dedupSugar skips one extra caller frame so LogErrorDedup reports the
	// caller's site rather than dedup.go.
	dedupSugar *zap.SugaredLogger

	lastSuppressionSummary time.Time

	// lastLogTime is when LogErrorDedup was last called, used to restart the
	// cycle after the error has been quiet for resetInterval.
	lastLogTime time.Time

	lastLoggedErrorMsg string

	summaryInterval time.Duration
	resetInterval   time.Duration

	// suppressedErrorCount counts repeats demoted to Debug since the last summary.
	suppressedErrorCount uint64

	errorSuppressionAnnounced bool

	// resetPending records that Reset was called; applied lazily on the next
	// LogErrorDedup call, and only if the message changed. See Reset.
	resetPending bool
}

// NewDedupLogger returns a DedupLogger writing to l with the default intervals.
func NewDedupLogger(l *zap.SugaredLogger) *DedupLogger {
	return &DedupLogger{
		SugaredLogger:   l,
		dedupSugar:      l.WithOptions(zap.AddCallerSkip(1)),
		summaryInterval: DefaultErrorSuppressionSummaryInterval,
		resetInterval:   DefaultErrorDedupResetInterval,
	}
}

// LogErrorDedup logs an error without spamming: first occurrence and first
// repeat at Error (the latter with a suppression note), further repeats at Debug
// plus a periodic Error summary of the suppressed count. A changed message or a
// quiet period of resetInterval ends the cycle with a final summary.
func (d *DedupLogger) LogErrorDedup(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	now := time.Now()

	// Quiet long enough: treat the error as cleared and start over.
	if !d.lastLogTime.IsZero() && now.Sub(d.lastLogTime) >= d.resetInterval {
		d.endCycle()
	}

	// Honor a pending Reset only when the message changed; a repeat of the same
	// message means the success signal was spurious.
	if d.resetPending {
		if msg != d.lastLoggedErrorMsg {
			d.endCycle()
		}
		d.resetPending = false
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

// emitSuppressionSummary logs a final Error summary of suppressed repeats.
// No-op when nothing was suppressed since the last summary.
func (d *DedupLogger) emitSuppressionSummary() {
	if d.suppressedErrorCount == 0 {
		return
	}
	d.dedupSugar.Errorf("previous error cleared or changed after %d further suppressed repeats: %s",
		d.suppressedErrorCount, d.lastLoggedErrorMsg)
	d.suppressedErrorCount = 0
}

// Reset signals that the caller considers the current error cleared (e.g. a
// reconcile loop that just succeeded). Applied lazily on the next LogErrorDedup
// call, and only if the message changed, so an unreliable success signal
// reported every tick while the same error keeps firing does not restart the
// cycle. A genuinely cleared error is also ended by the quiet resetInterval.
func (d *DedupLogger) Reset() {
	d.resetPending = true
}

// endCycle emits a final summary and clears message identity so the next call
// starts a fresh cycle.
func (d *DedupLogger) endCycle() {
	d.emitSuppressionSummary()
	d.lastLoggedErrorMsg = ""
	d.errorSuppressionAnnounced = false
	d.resetPending = false
}
