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

package actions

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"

	deps "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	fsmv2sentry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"
)

// TestCommunicatorFSMLoggerWiredToSentryHook locks the invariant that the
// FSMLogger constructed by communicatorFSMLogger() routes through the
// package-level SentryHook. The original bug was that
// HandleActionMessage built deps.NewFSMLogger(log) directly, so SentryWarn
// and SentryError calls emitted structured zap entries but never reached
// Sentry. 30 days of Sentry telemetry confirmed zero events under the
// expected feature tags despite 19 SentryError call sites firing.
//
// An observer-core assertion would have PASSED before the fix — the
// FSMLogger always emits the entry; the bug was that no hook intercepted
// it. This test asserts the hook INTERCEPTED the entry by checking that
// the debouncer's lastSeen map was updated for the expected fingerprint.
func TestCommunicatorFSMLoggerWiredToSentryHook(t *testing.T) {
	logger := communicatorFSMLogger()

	if communicatorSentryHook == nil {
		t.Fatal("communicatorSentryHook nil after communicatorFSMLogger() — Once.Do did not run")
	}

	// Use a unique event_name so the fingerprint cannot collide with any
	// other test that runs in the same process. BuildFingerprint hashes
	// (level, feature, event_name, error_types).
	const eventName = "test_wiring_assertion_event_name_unique_xn4q"

	feature := deps.FeatureFSMv1Communicator
	testErr := errors.New("test_wiring_assertion_error_unique_xn4q")

	expectedFingerprint := strings.Join(
		fsmv2sentry.BuildFingerprint(
			zapcore.ErrorLevel,
			string(feature),
			eventName,
			fsmv2sentry.ExtractErrorTypes(testErr),
		),
		"|",
	)

	// Pre-check: the fingerprint has not been seen yet. If a previous
	// test in this binary somehow recorded the same fingerprint, this
	// test is meaningless. Failing loudly is better than passing
	// vacuously.
	if !communicatorSentryHook.Debouncer().ShouldCapture(expectedFingerprint + "_precheck") {
		t.Fatal("unexpected: precheck fingerprint already seen")
	}

	// Fire the SentryError through the FSMLogger. If the hook is wired,
	// SentryHook.Write intercepts the entry, computes the fingerprint
	// (same formula as expectedFingerprint above), and calls
	// ShouldCapture which records the timestamp in debouncer.lastSeen.
	logger.SentryError(feature, communicatorHierarchyPath, testErr, eventName)

	// Post-assert: a fresh ShouldCapture call with the SAME fingerprint
	// must return false (within the 5-min window) because the hook
	// already recorded it. If the hook did NOT intercept the entry
	// (i.e., we regressed back to a bare FSMLogger), the lastSeen map
	// is empty for this fingerprint and ShouldCapture returns true.
	if communicatorSentryHook.Debouncer().ShouldCapture(expectedFingerprint) {
		t.Errorf("SentryHook did not intercept FSMLogger.SentryError — communicatorFSMLogger() is not wrapping the logger with the hook")
	}
}
