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
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// panickingAction is a minimal Action used in tests to drive HandleActionMessage
// down a panicking path. Each lifecycle method can be configured to panic with
// an arbitrary value.
type panickingAction struct {
	panicOnParse    any
	panicOnValidate any
	panicOnExecute  any
}

func (a *panickingAction) Parse(_ interface{}) error {
	if a.panicOnParse != nil {
		panic(a.panicOnParse)
	}

	return nil
}

func (a *panickingAction) Validate() error {
	if a.panicOnValidate != nil {
		panic(a.panicOnValidate)
	}

	return nil
}

func (a *panickingAction) Execute() (interface{}, map[string]interface{}, error) {
	if a.panicOnExecute != nil {
		panic(a.panicOnExecute)
	}

	return nil, nil, nil
}

func (a *panickingAction) getUserEmail() string { return "test@example.com" }
func (a *panickingAction) getUuid() uuid.UUID   { return uuid.Nil }

// captureStderr redirects os.Stderr for the duration of fn and returns the
// captured output.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stderr

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	// Panic-safe restore: if fn() panics, the deferred restore keeps
	// os.Stderr from staying redirected and the reader goroutine from
	// blocking forever on io.ReadAll. t.Cleanup belt-and-braces for the
	// case where the panic kills the test process before defer fires.
	os.Stderr = w

	t.Cleanup(func() { os.Stderr = orig })

	defer func() {
		os.Stderr = orig
		_ = w.Close()
	}()

	done := make(chan string, 1)

	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	// Close before the deferred restore so the reader's io.ReadAll
	// returns. The deferred close on a panic path is a redundant safety
	// net (io.ReadAll on an already-closed reader returns nil, nil).
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}

	return <-done
}

// panickingStringer's String() always panics. Used to verify
// classifyActionPanic does not invoke String() on the recovered value.
type panickingStringer struct{}

func (panickingStringer) String() string { panic("stringer boom") }

// =============================================================================
// classifyActionPanic (T2.7, T2.8, T2.9)
// =============================================================================

func TestClassifyActionPanic(t *testing.T) {
	// T2.7 — error values map to error_panic, including runtime.Error
	t.Run("error value", func(t *testing.T) {
		gotType, gotErr := classifyActionPanic(errors.New("boom"))
		if gotType != "error_panic" {
			t.Errorf("want error_panic, got %q", gotType)
		}

		if gotErr == nil || gotErr.Error() != "boom" {
			t.Errorf("want err=boom, got %v", gotErr)
		}
	})

	// T2.8 — string values map to string_panic
	t.Run("string value", func(t *testing.T) {
		gotType, gotErr := classifyActionPanic("oops")
		if gotType != "string_panic" {
			t.Errorf("want string_panic, got %q", gotType)
		}

		if gotErr == nil || gotErr.Error() != "oops" {
			t.Errorf("want err=oops, got %v", gotErr)
		}
	})

	// T2.9 — anything else maps to unknown_panic. The error renders %T to
	// avoid calling String() on a pathological Stringer.
	t.Run("arbitrary value", func(t *testing.T) {
		gotType, gotErr := classifyActionPanic(42)
		if gotType != "unknown_panic" {
			t.Errorf("want unknown_panic, got %q", gotType)
		}

		if gotErr == nil || !strings.Contains(gotErr.Error(), "int") {
			t.Errorf("want err to mention type 'int', got %v", gotErr)
		}
	})

	// T2.9b — a Stringer whose String() panics must not re-panic during
	// classification. The fix uses %T (static type) instead of %v.
	t.Run("panicking stringer", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("classifyActionPanic re-panicked on bad Stringer: %v", r)
			}
		}()

		gotType, gotErr := classifyActionPanic(panickingStringer{})
		if gotType != "unknown_panic" {
			t.Errorf("want unknown_panic, got %q", gotType)
		}

		if gotErr == nil {
			t.Errorf("want non-nil err, got nil")
		}
	})
}

// =============================================================================
// recoverActionPanic — direct invocation (T2.3, T2.4, T2.5, T2.6)
// =============================================================================

func TestRecoverActionPanicIncrementsMetric(t *testing.T) {
	// T2.4 — metric is recorded with the right labels
	payload := models.ActionMessagePayload{ActionType: "edit-protocol-converter", ActionUUID: uuid.New()}
	out := make(chan *models.UMHMessage, 1)
	log := logger.For(logger.ComponentCommunicator)

	before := metrics.ActionPanicsTotalForTest(string(payload.ActionType), "error_panic")
	recoverActionPanic(errors.New("simulated"), uuid.New(), "user@example.com", payload, deps.NewFSMLogger(log), out)
	after := metrics.ActionPanicsTotalForTest(string(payload.ActionType), "error_panic")

	if after-before != 1 {
		t.Fatalf("counter not incremented: before=%v after=%v", before, after)
	}
}

func TestRecoverActionPanicSendsFailureReply(t *testing.T) {
	// T2.2 — reply is sent with the right ActionUUID and failure state
	actionUUID := uuid.New()
	payload := models.ActionMessagePayload{ActionType: "edit-protocol-converter", ActionUUID: actionUUID}
	out := make(chan *models.UMHMessage, 1)
	log := logger.For(logger.ComponentCommunicator)

	recoverActionPanic(errors.New("simulated"), uuid.New(), "user@example.com", payload, deps.NewFSMLogger(log), out)

	select {
	case msg := <-out:
		dec, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
		if err != nil {
			t.Fatalf("decode reply: %v", err)
		}

		reply, ok := dec.Payload.(map[string]interface{})
		if !ok {
			t.Fatalf("payload is %T not map[string]interface{}", dec.Payload)
		}

		if got, _ := reply["actionUUID"].(string); got != actionUUID.String() {
			t.Errorf("want actionUUID=%v, got %v", actionUUID, got)
		}

		if got, _ := reply["actionReplyState"].(string); got != string(models.ActionFinishedWithFailure) {
			t.Errorf("want state=%v, got %v", models.ActionFinishedWithFailure, got)
		}

		if payload, _ := reply["actionReplyPayload"].(string); !strings.Contains(payload, actionUUID.String()) {
			t.Errorf("want actionReplyPayload to contain ActionUUID; got %q", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no reply received within timeout")
	}
}

func TestRecoverActionPanicNonBlockingOnFullChannel(t *testing.T) {
	// T2.5 — when outbound is full, recovery completes without blocking and
	// writes a "reply dropped" line to stderr
	payload := models.ActionMessagePayload{ActionType: "edit-protocol-converter", ActionUUID: uuid.New()}

	full := make(chan *models.UMHMessage, 1)
	full <- &models.UMHMessage{} // saturate

	log := logger.For(logger.ComponentCommunicator)

	done := make(chan struct{})
	stderr := captureStderr(t, func() {
		go func() {
			recoverActionPanic(errors.New("simulated"), uuid.New(), "user@example.com", payload, deps.NewFSMLogger(log), full)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("recoverActionPanic blocked on full channel")
		}
	})

	if !strings.Contains(stderr, "action_handler_panic_reply_dropped") {
		t.Errorf("expected stderr to contain action_handler_panic_reply_dropped; got %q", stderr)
	}
}

func TestRecoverActionPanicDoublePanicGuard(t *testing.T) {
	// T2.3 — if SentryError itself panics, recoverActionPanic swallows the
	// secondary panic and writes a stderr fallback line.
	payload := models.ActionMessagePayload{ActionType: "edit-protocol-converter", ActionUUID: uuid.New()}
	out := make(chan *models.UMHMessage, 1)
	bad := &panickingFSMLogger{}

	stderr := captureStderr(t, func() {
		// If the double-panic guard works, this returns normally instead of
		// propagating "logger boom" up.
		recoverActionPanic(errors.New("primary"), uuid.New(), "user@example.com", payload, bad, out)
	})

	if !strings.Contains(stderr, "action_handler_double_panic") {
		t.Errorf("expected stderr to contain action_handler_double_panic; got %q", stderr)
	}
}

// countingPanickingFSMLogger panics on the first SentryError invocation, then
// records subsequent calls. Used to assert that the double-panic guard
// re-attempts a Sentry breadcrumb after the primary Sentry call fails.
type countingPanickingFSMLogger struct {
	calls []capturedSentryError
}

func (l *countingPanickingFSMLogger) Debug(string, ...deps.Field)                            {}
func (l *countingPanickingFSMLogger) Info(string, ...deps.Field)                             {}
func (l *countingPanickingFSMLogger) SentryWarn(deps.Feature, string, string, ...deps.Field) {}
func (l *countingPanickingFSMLogger) SentryError(feature deps.Feature, hp string, err error, msg string, fields ...deps.Field) {
	l.calls = append(l.calls, capturedSentryError{feature, hp, err, msg, fields})
	if len(l.calls) == 1 {
		panic("primary sentry boom")
	}
}
func (l *countingPanickingFSMLogger) With(...deps.Field) deps.FSMLogger { return l }

func TestRecoverActionPanicDoublePanicGuardFiresWhenMetricPanics(t *testing.T) {
	// Cover the second of the two secondary-panic sites in recoverActionPanic:
	// the metric counter. Without the recordActionPanicFn seam this branch
	// was untested, leaving a silent-failure gap if a future refactor wraps
	// the counter in a layer that can panic.
	payload := models.ActionMessagePayload{ActionType: "edit-protocol-converter", ActionUUID: uuid.New()}
	out := make(chan *models.UMHMessage, 1)
	log := logger.For(logger.ComponentCommunicator)

	prev := recordActionPanicFn

	t.Cleanup(func() { recordActionPanicFn = prev })

	recordActionPanicFn = func(string, string) { panic("metric boom") }

	stderr := captureStderr(t, func() {
		recoverActionPanic(errors.New("primary"), uuid.New(), "user@example.com", payload, deps.NewFSMLogger(log), out)
	})

	if !strings.Contains(stderr, "action_handler_double_panic") {
		t.Errorf("expected stderr to contain action_handler_double_panic; got %q", stderr)
	}
}

func TestRecoverActionPanicDoublePanicGuardReattemptsSentry(t *testing.T) {
	// Mirror of pkg/fsmv2/supervisor/reconciliation.go:518-528 — when the
	// primary Sentry call panics, the double-panic guard must still attempt
	// to record a Sentry breadcrumb so future nil-fsmLogger-class regressions
	// remain visible to engineering.
	payload := models.ActionMessagePayload{ActionType: "edit-protocol-converter", ActionUUID: uuid.New()}
	out := make(chan *models.UMHMessage, 1)
	bad := &countingPanickingFSMLogger{}

	stderr := captureStderr(t, func() {
		recoverActionPanic(errors.New("primary"), uuid.New(), "user@example.com", payload, bad, out)
	})

	if !strings.Contains(stderr, "action_handler_double_panic") {
		t.Errorf("expected stderr to contain action_handler_double_panic; got %q", stderr)
	}

	if len(bad.calls) < 2 {
		t.Fatalf("want at least 2 SentryError calls (primary + double-panic), got %d", len(bad.calls))
	}

	secondary := bad.calls[1]
	if secondary.msg != "action_handler_double_panic" {
		t.Errorf("want secondary msg=action_handler_double_panic, got %q", secondary.msg)
	}

	if secondary.feature != deps.FeatureFSMv1Communicator {
		t.Errorf("want secondary feature=%v, got %v", deps.FeatureFSMv1Communicator, secondary.feature)
	}
}

// capturingFSMLogger records SentryError calls so tests can assert on the
// field set passed to Sentry.
type capturingFSMLogger struct {
	calls []capturedSentryError
}

type capturedSentryError struct {
	feature       deps.Feature
	hierarchyPath string
	err           error
	msg           string
	fields        []deps.Field
}

func (l *capturingFSMLogger) Debug(string, ...deps.Field)                            {}
func (l *capturingFSMLogger) Info(string, ...deps.Field)                             {}
func (l *capturingFSMLogger) SentryWarn(deps.Feature, string, string, ...deps.Field) {}
func (l *capturingFSMLogger) SentryError(feature deps.Feature, hp string, err error, msg string, fields ...deps.Field) {
	l.calls = append(l.calls, capturedSentryError{feature, hp, err, msg, fields})
}
func (l *capturingFSMLogger) With(...deps.Field) deps.FSMLogger { return l }

func TestRecoverActionPanicLogsSentryFields(t *testing.T) {
	// T2.6 — Sentry event "action_handler_panic" is logged with the required
	// fields, and stack_trace is non-empty (debug.Stack() caveat noted in
	// recoverActionPanic).
	actionUUID := uuid.New()
	payload := models.ActionMessagePayload{ActionType: "edit-protocol-converter", ActionUUID: actionUUID}
	out := make(chan *models.UMHMessage, 1)
	cap := &capturingFSMLogger{}

	recoverActionPanic(errors.New("simulated"), uuid.New(), "user@example.com", payload, cap, out)

	if len(cap.calls) != 1 {
		t.Fatalf("expected 1 SentryError call, got %d", len(cap.calls))
	}

	call := cap.calls[0]
	if call.feature != deps.FeatureFSMv1Communicator {
		t.Errorf("want feature=%v, got %v", deps.FeatureFSMv1Communicator, call.feature)
	}

	// Hierarchy path drives fsm_version/worker_type/worker_chain in the Sentry
	// hook. Dotted FSMv1 format ensures events tag as fsm_version=v1, matching
	// the convention from pkg/config/manager.go's configManagerHierarchyPath.
	if call.hierarchyPath != "fsmv1.Communicator" {
		t.Errorf("want hierarchyPath=fsmv1.Communicator, got %q", call.hierarchyPath)
	}

	if call.msg != "action_handler_panic" {
		t.Errorf("want msg=action_handler_panic, got %q", call.msg)
	}

	want := map[string]string{
		"action_type": string(payload.ActionType),
		"action_uuid": actionUUID.String(),
		"panic_type":  "error_panic",
	}
	got := map[string]string{}

	var stackTraceLen int

	for _, f := range call.fields {
		if s, ok := f.Value.(string); ok {
			if f.Key == "stack_trace" {
				stackTraceLen = len(s)
			} else {
				got[f.Key] = s
			}
		}
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %q: want %q, got %q", k, v, got[k])
		}
	}

	if stackTraceLen == 0 {
		t.Errorf("want non-empty stack_trace field, got empty")
	}
}

// panickingFSMLogger satisfies deps.FSMLogger but panics on SentryError. Used
// to exercise the double-panic guard.
type panickingFSMLogger struct{}

func (panickingFSMLogger) Debug(string, ...deps.Field)                            {}
func (panickingFSMLogger) Info(string, ...deps.Field)                             {}
func (panickingFSMLogger) SentryWarn(deps.Feature, string, string, ...deps.Field) {}
func (panickingFSMLogger) SentryError(deps.Feature, string, error, string, ...deps.Field) {
	panic("logger boom")
}
func (l panickingFSMLogger) With(...deps.Field) deps.FSMLogger { return l }

// =============================================================================
// HandleActionMessage end-to-end (T2.1)
// =============================================================================

func TestHandleActionMessageRecoversFromPanic(t *testing.T) {
	// T2.1 — driving a payload through a panicking action does not crash the
	// process, AND the recovery path actually fires: a failure reply reaches
	// the outbound channel and the panic counter ticks. Non-hang alone would
	// pass even if a future refactor silently swallowed the panic without
	// invoking recoverActionPanic.
	prev := newActionFromPayloadFn

	t.Cleanup(func() { newActionFromPayloadFn = prev })

	newActionFromPayloadFn = func(_ uuid.UUID, _ models.ActionMessagePayload, _ string, _ chan *models.UMHMessage, _ *fsm.SnapshotManager, _ config.ConfigManager, _ *zap.SugaredLogger, _ deps.FSMLogger) Action {
		return &panickingAction{panicOnParse: errors.New("parse boom")}
	}

	out := make(chan *models.UMHMessage, 8)
	actionUUID := uuid.New()

	beforePanics := metrics.ActionPanicsTotalForTest("edit-protocol-converter", "error_panic")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		HandleActionMessage(
			uuid.New(),
			models.ActionMessagePayload{ActionType: models.EditProtocolConverter, ActionUUID: actionUUID},
			"user@example.com",
			out,
			"", nil, uuid.Nil, nil, nil,
		)
	}()

	doneCh := make(chan struct{})

	go func() { wg.Wait(); close(doneCh) }()

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("HandleActionMessage did not return — panic recovery is broken")
	}

	afterPanics := metrics.ActionPanicsTotalForTest("edit-protocol-converter", "error_panic")
	if afterPanics-beforePanics < 1 {
		t.Errorf("panic counter did not advance: before=%v after=%v — recoverActionPanic did not fire", beforePanics, afterPanics)
	}

	// Drain replies and check at least one carries ActionFinishedWithFailure.
	// Without this, a future refactor that recovers but skips the reply path
	// would still pass the non-hang assertion above.
	var sawFailureReply bool

	for len(out) > 0 {
		msg := <-out
		if msg == nil {
			continue
		}

		dec, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
		if err != nil {
			continue
		}

		reply, ok := dec.Payload.(map[string]interface{})
		if !ok {
			continue
		}

		if state, _ := reply["actionReplyState"].(string); state == string(models.ActionFinishedWithFailure) {
			sawFailureReply = true

			break
		}
	}

	if !sawFailureReply {
		t.Errorf("no ActionFinishedWithFailure reply observed — recovery path skipped the user-visible failure")
	}
}
