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

package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// recordedLogCall is one recorded logger call.
type recordedLogCall struct {
	Event  string
	Err    error
	Fields []deps.Field
}

// recordingSentryLogger records every logger call at warn level or above; a
// recording method added to it records too.
type recordingSentryLogger struct {
	mu           sync.Mutex
	warns        []recordedLogCall
	sentryWarns  []recordedLogCall
	sentryErrors []recordedLogCall
}

func newRecordingSentryLogger() *recordingSentryLogger {
	return &recordingSentryLogger{}
}

func (r *recordingSentryLogger) Debug(string, ...deps.Field) {}

func (r *recordingSentryLogger) Info(string, ...deps.Field) {}

func (r *recordingSentryLogger) Warn(event string, fields ...deps.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.warns = append(r.warns, recordedLogCall{Event: event, Fields: fields})
}

func (r *recordingSentryLogger) SentryWarn(_ deps.Feature, _ string, event string, fields ...deps.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sentryWarns = append(r.sentryWarns, recordedLogCall{Event: event, Fields: fields})
}

func (r *recordingSentryLogger) SentryError(_ deps.Feature, _ string, err error, event string, fields ...deps.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sentryErrors = append(r.sentryErrors, recordedLogCall{Event: event, Err: err, Fields: fields})
}

// With returns the logger itself, so a supervisor and the child supervisors
// built from its base logger record into the same list.
func (r *recordingSentryLogger) With(...deps.Field) deps.FSMLogger { return r }

func (r *recordingSentryLogger) warnCount(event string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, call := range r.warns {
		if call.Event == event {
			count++
		}
	}

	return count
}

func (r *recordingSentryLogger) sentryErrorEvents() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]string, 0, len(r.sentryErrors))
	for _, call := range r.sentryErrors {
		events = append(events, call.Event)
	}

	return events
}

func (r *recordingSentryLogger) sentryWarnEvents() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]string, 0, len(r.sentryWarns))
	for _, call := range r.sentryWarns {
		events = append(events, call.Event)
	}

	return events
}

// switchableDeriveWorker derives a running desired state until its err field
// is set, then returns that error from every derive call.
type switchableDeriveWorker struct {
	err error
}

func (w *switchableDeriveWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "test-worker", CollectedAt: time.Now()}, nil
}

func (w *switchableDeriveWorker) DeriveDesiredState(any) (fsmv2.DesiredState, error) {
	if w.err != nil {
		return nil, w.err
	}

	return &config.DesiredState{State: "running"}, nil
}

func (w *switchableDeriveWorker) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

// configTextDeriveWorker turns the user spec's config text into a derive
// result: text containing "{{" is a *config.ConfigError whose message is the
// text itself, so two different texts produce two different messages.
type configTextDeriveWorker struct{}

func (w *configTextDeriveWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "cfgerr-child-001", CollectedAt: time.Now()}, nil
}

func (w *configTextDeriveWorker) DeriveDesiredState(spec any) (fsmv2.DesiredState, error) {
	cfg := ""
	if us, ok := spec.(config.UserSpec); ok {
		cfg = us.Config
	}

	if strings.Contains(cfg, "{{") {
		return nil, &config.ConfigError{Field: "config", Err: errors.New(cfg)}
	}

	return &config.DesiredState{State: "running"}, nil
}

func (w *configTextDeriveWorker) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

// childSpecParentWorker renders one child spec whose config text is the
// parent user spec's config text, so a test drives the child's config through
// TestUpdateUserSpec. childWorkerType picks the child's worker type.
type childSpecParentWorker struct {
	childWorkerType string
}

func (w *childSpecParentWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "parent-worker", CollectedAt: time.Now()}, nil
}

func (w *childSpecParentWorker) DeriveDesiredState(spec any) (fsmv2.DesiredState, error) {
	cfg := ""
	if us, ok := spec.(config.UserSpec); ok {
		cfg = us.Config
	}

	return &config.DesiredState{
		State: "running",
		ChildrenSpecs: []config.ChildSpec{{
			Name:       "cfgerr-child",
			WorkerType: w.childWorkerType,
			UserSpec:   config.UserSpec{Config: cfg},
		}},
	}, nil
}

func (w *childSpecParentWorker) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

// twoChildSpecParentWorker renders two child specs with the same config text,
// so a config error in it fails both children with the same message.
type twoChildSpecParentWorker struct {
	childWorkerType string
}

func (w *twoChildSpecParentWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "parent-worker", CollectedAt: time.Now()}, nil
}

func (w *twoChildSpecParentWorker) DeriveDesiredState(spec any) (fsmv2.DesiredState, error) {
	cfg := ""
	if us, ok := spec.(config.UserSpec); ok {
		cfg = us.Config
	}

	return &config.DesiredState{
		State: "running",
		ChildrenSpecs: []config.ChildSpec{
			{Name: "cfgerr-child-a", WorkerType: w.childWorkerType, UserSpec: config.UserSpec{Config: cfg}},
			{Name: "cfgerr-child-b", WorkerType: w.childWorkerType, UserSpec: config.UserSpec{Config: cfg}},
		},
	}, nil
}

func (w *twoChildSpecParentWorker) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

// plainErrorDeriveWorker returns a plain, non-config error from every derive.
type plainErrorDeriveWorker struct{}

func (w *plainErrorDeriveWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "plain-error-child", CollectedAt: time.Now()}, nil
}

func (w *plainErrorDeriveWorker) DeriveDesiredState(any) (fsmv2.DesiredState, error) {
	return nil, errors.New("plain derive failure")
}

func (w *plainErrorDeriveWorker) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

var _ = Describe("Supervisor config errors", func() {
	ctx := context.Background()

	Describe("AddWorker with a config error", func() {
		It("creates no worker, returns the config error and stays out of Sentry", func() {
			recorder := newRecordingSentryLogger()
			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "test",
				Store:      newMockTriangularStore(),
				Logger:     recorder,
			})

			err := s.AddWorker(mockIdentity(), &switchableDeriveWorker{
				err: &config.ConfigError{Field: "read", Err: errors.New("x")},
			})
			Expect(err).To(HaveOccurred())

			var cfgErr *config.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())

			Expect(s.ListWorkers()).To(BeEmpty())

			Expect(recorder.sentryErrorEvents()).To(BeEmpty())
			Expect(recorder.sentryWarnEvents()).To(BeEmpty())
		})
	})

	Describe("AddWorker with a plain derive error", func() {
		It("still reaches Sentry as worker_add_derive_desired_failed", func() {
			recorder := newRecordingSentryLogger()
			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "test",
				Store:      newMockTriangularStore(),
				Logger:     recorder,
			})

			err := s.AddWorker(mockIdentity(), &switchableDeriveWorker{err: errors.New("boom")})
			Expect(err).To(HaveOccurred())

			Expect(s.ListWorkers()).To(BeEmpty())

			Expect(recorder.sentryErrorEvents()).To(ConsistOf("worker_add_derive_desired_failed"))
			Expect(recorder.warnCount("config_error")).To(Equal(0))
		})
	})

	Describe("A parent whose child has a config error", func() {
		const (
			brokenChildConfig = "{{ first mistake"
			otherBrokenConfig = "{{ second mistake"
			validChildConfig  = "state: running"
		)

		var (
			recorder    *recordingSentryLogger
			parentSuper *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		)

		BeforeEach(func() {
			recorder = newRecordingSentryLogger()

			err := factory.RegisterFactoryByType("configerrorchild",
				func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &configTextDeriveWorker{}
				})
			Expect(err).NotTo(HaveOccurred())

			err = factory.RegisterSupervisorFactoryByType("configerrorchild", func(cfg any) any {
				return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
			})
			Expect(err).NotTo(HaveOccurred())

			parentSuper = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Store:      newMockTriangularStore(),
				Logger:     recorder,
			})

			err = parentSuper.AddWorker(deps.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}, &childSpecParentWorker{childWorkerType: "configerrorchild"})
			Expect(err).NotTo(HaveOccurred())

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: brokenChildConfig})
		})

		It("warns once per distinct message, creates the child once fixed, and stays out of Sentry", func() {
			for i := 0; i < 5; i++ {
				Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			}

			Expect(recorder.warnCount("config_error")).To(Equal(1))
			Expect(parentSuper.GetChildren()).To(BeEmpty())

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: validChildConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.GetChildren()).To(HaveKey("cfgerr-child"))

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: otherBrokenConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())

			Expect(recorder.warnCount("config_error")).To(Equal(2))
			Expect(recorder.sentryErrorEvents()).To(BeEmpty())
			Expect(recorder.sentryWarnEvents()).To(BeEmpty())

			// Creating the child clears its dedup entry, so the same message warns again.
			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: validChildConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.GetChildren()).To(HaveKey("cfgerr-child"))

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: otherBrokenConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())

			Expect(recorder.warnCount("config_error")).To(Equal(3))
		})
	})

	Describe("A parent whose two children share a config error", func() {
		It("warns once per child even for the same message", func() {
			recorder := newRecordingSentryLogger()

			err := factory.RegisterFactoryByType("configerrorchild",
				func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &configTextDeriveWorker{}
				})
			Expect(err).NotTo(HaveOccurred())

			err = factory.RegisterSupervisorFactoryByType("configerrorchild", func(cfg any) any {
				return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
			})
			Expect(err).NotTo(HaveOccurred())

			parentSuper := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Store:      newMockTriangularStore(),
				Logger:     recorder,
			})

			err = parentSuper.AddWorker(deps.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}, &twoChildSpecParentWorker{childWorkerType: "configerrorchild"})
			Expect(err).NotTo(HaveOccurred())

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: "{{ first mistake"})
			for i := 0; i < 2; i++ {
				Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			}

			Expect(recorder.warnCount("config_error")).To(Equal(2))
			Expect(parentSuper.GetChildren()).To(BeEmpty())
			Expect(recorder.sentryErrorEvents()).To(BeEmpty())
			Expect(recorder.sentryWarnEvents()).To(BeEmpty())
		})
	})

	Describe("A parent whose child fails with a plain derive error", func() {
		const plainErrorChildConfig = "state: running"

		var (
			recorder    *recordingSentryLogger
			parentSuper *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		)

		BeforeEach(func() {
			recorder = newRecordingSentryLogger()

			err := factory.RegisterFactoryByType("plainerrorchild",
				func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &plainErrorDeriveWorker{}
				})
			Expect(err).NotTo(HaveOccurred())

			err = factory.RegisterSupervisorFactoryByType("plainerrorchild", func(cfg any) any {
				return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
			})
			Expect(err).NotTo(HaveOccurred())

			parentSuper = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Store:      newMockTriangularStore(),
				Logger:     recorder,
			})

			err = parentSuper.AddWorker(deps.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}, &childSpecParentWorker{childWorkerType: "plainerrorchild"})
			Expect(err).NotTo(HaveOccurred())

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: plainErrorChildConfig})
		})

		It("logs child_supervisor_add_worker_failed and stays out of config handling", func() {
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())

			Expect(parentSuper.GetChildren()).To(BeEmpty())

			Expect(recorder.sentryErrorEvents()).To(ConsistOf(
				"worker_add_derive_desired_failed",
				"child_supervisor_add_worker_failed",
			))
			Expect(recorder.warnCount("config_error")).To(Equal(0))
		})
	})

	Describe("A running worker whose spec breaks", func() {
		It("warns once, stays out of Sentry and keeps the stored desired state", func() {
			recorder := newRecordingSentryLogger()
			store := newMockTriangularStore()
			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "test",
				Store:      store,
				Logger:     recorder,
			})

			Expect(s.AddWorker(mockIdentity(), &configTextDeriveWorker{})).NotTo(HaveOccurred())

			s.TestUpdateUserSpec(config.UserSpec{Config: "valid"})
			Expect(s.TestTick(ctx)).NotTo(HaveOccurred())

			desiredBefore, err := store.LoadDesired(ctx, "test", "test-worker")
			Expect(err).NotTo(HaveOccurred())

			s.TestUpdateUserSpec(config.UserSpec{Config: "{{ misspelled name"})
			for i := 0; i < 3; i++ {
				Expect(s.TestTick(ctx)).To(HaveOccurred())
			}

			Expect(recorder.warnCount("config_error")).To(Equal(1))
			Expect(recorder.sentryErrorEvents()).To(BeEmpty())

			desiredAfter, err := store.LoadDesired(ctx, "test", "test-worker")
			Expect(err).NotTo(HaveOccurred())

			beforeJSON, err := json.Marshal(desiredBefore)
			Expect(err).NotTo(HaveOccurred())
			afterJSON, err := json.Marshal(desiredAfter)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(afterJSON)).To(Equal(string(beforeJSON)))

			// A successful derive clears the memory, so the same message warns again.
			s.TestUpdateUserSpec(config.UserSpec{Config: "valid"})
			Expect(s.TestTick(ctx)).NotTo(HaveOccurred())

			s.TestUpdateUserSpec(config.UserSpec{Config: "{{ misspelled name"})
			for i := 0; i < 3; i++ {
				Expect(s.TestTick(ctx)).To(HaveOccurred())
			}

			Expect(recorder.warnCount("config_error")).To(Equal(2))
			Expect(recorder.sentryErrorEvents()).To(BeEmpty())
		})
	})

	Describe("A plain derive error", func() {
		It("still reaches Sentry as derive_desired_state_failed", func() {
			recorder := newRecordingSentryLogger()
			s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "test",
				Store:      newMockTriangularStore(),
				Logger:     recorder,
			})

			worker := &switchableDeriveWorker{}
			Expect(s.AddWorker(mockIdentity(), worker)).NotTo(HaveOccurred())

			worker.err = errors.New("boom")
			Expect(s.TestTick(ctx)).To(HaveOccurred())

			Expect(recorder.sentryErrorEvents()).To(ConsistOf("derive_desired_state_failed"))
			Expect(recorder.warnCount("config_error")).To(Equal(0))
		})
	})
})
