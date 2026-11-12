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
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor"
)

// recordedLogCall is one recorded logger call.
type recordedLogCall struct {
	Event  string
	Err    error
	Fields []deps.Field
}

// recordingSentryLogger records every Info call and every call at warn level
// or above.
type recordingSentryLogger struct {
	mu           sync.Mutex
	infos        []recordedLogCall
	sentryWarns  []recordedLogCall
	sentryErrors []recordedLogCall
}

func newRecordingSentryLogger() *recordingSentryLogger {
	return &recordingSentryLogger{}
}

func (r *recordingSentryLogger) Debug(string, ...deps.Field) {}

func (r *recordingSentryLogger) Info(event string, fields ...deps.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.infos = append(r.infos, recordedLogCall{Event: event, Fields: fields})
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

// infoCalls returns the recorded Info calls named event, in call order.
func (r *recordingSentryLogger) infoCalls(event string) []recordedLogCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	var calls []recordedLogCall
	for _, call := range r.infos {
		if call.Event == event {
			calls = append(calls, call)
		}
	}

	return calls
}

func (r *recordingSentryLogger) infoCount(event string) int {
	return len(r.infoCalls(event))
}

// expectNoConfigErrorWarning fails when "config_error" was logged at warn
// level or above. A config mistake is the user's, so it is logged at Info.
func expectNoConfigErrorWarning(r *recordingSentryLogger) {
	GinkgoHelper()

	Expect(r.sentryWarnEvents()).NotTo(ContainElement("config_error"),
		"a config mistake must not be logged as a warning")
	Expect(r.sentryErrorEvents()).NotTo(ContainElement("config_error"),
		"a config mistake must not be logged as an error")
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

// logFieldMap turns one recorded logger call into a map of its field values.
func logFieldMap(call recordedLogCall) map[string]any {
	fields := make(map[string]any, len(call.Fields))

	for _, f := range call.Fields {
		fields[f.Key] = f.Value
	}

	return fields
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

// stateTypoConfig is parsed by the real WorkerBase.DeriveDesiredState, so an
// unknown state value in the user spec runs the real validation.
//
//nolint:unused // instantiated as the config argument of WorkerBase below
type stateTypoConfig struct {
	config.BaseUserSpec `yaml:",inline"`
}

// stateTypoWorker derives through the promoted WorkerBase method, unlike the
// hand-built mocks above.
type stateTypoWorker struct {
	fsmv2.WorkerBase[stateTypoConfig, struct{}, struct{}]
}

func (w *stateTypoWorker) CollectObservedState(context.Context, fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: "state-typo-child", CollectedAt: time.Now()}, nil
}

func (w *stateTypoWorker) GetInitialState() fsmv2.State[any, any] { return &mockState{} }

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
			Expect(recorder.infoCount("config_error")).To(Equal(0))
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

		It("logs once per distinct message at Info, creates the child once fixed, and stays out of Sentry", func() {
			for i := 0; i < 5; i++ {
				Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			}

			Expect(recorder.infoCount("config_error")).To(Equal(1))
			Expect(parentSuper.GetChildren()).To(BeEmpty())

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: validChildConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.GetChildren()).To(HaveKey("cfgerr-child"))

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: otherBrokenConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())

			Expect(recorder.infoCount("config_error")).To(Equal(2))
			Expect(recorder.sentryErrorEvents()).To(BeEmpty())
			Expect(recorder.sentryWarnEvents()).To(BeEmpty())
			expectNoConfigErrorWarning(recorder)

			// Creating the child clears its dedup entry, so the same message is logged again.
			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: validChildConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.GetChildren()).To(HaveKey("cfgerr-child"))

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: otherBrokenConfig})
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())

			Expect(recorder.infoCount("config_error")).To(Equal(3))
		})
	})

	Describe("A parent whose two children share a config error", func() {
		It("logs once per child even for the same message", func() {
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

			Expect(recorder.infoCount("config_error")).To(Equal(2))
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
			Expect(recorder.infoCount("config_error")).To(Equal(0))
		})
	})

	Describe("A running worker whose spec breaks", func() {
		It("logs once at Info, stays out of Sentry and keeps the stored desired state", func() {
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

			Expect(recorder.infoCount("config_error")).To(Equal(1))
			Expect(recorder.sentryErrorEvents()).To(BeEmpty())
			expectNoConfigErrorWarning(recorder)

			ownLogFields := logFieldMap(recorder.infoCalls("config_error")[0])
			Expect(ownLogFields["field"]).To(Equal("config"),
				"the log line names the config error's field")
			Expect(fmt.Sprint(ownLogFields["error"])).To(ContainSubstring("misspelled"),
				"the log line carries the underlying error")
			Expect(ownLogFields["message"]).To(ContainSubstring("misspelled"))
			Expect(ownLogFields["hierarchy_path"]).To(ContainSubstring("test-worker"),
				"the log line names the worker it is about")

			desiredAfter, err := store.LoadDesired(ctx, "test", "test-worker")
			Expect(err).NotTo(HaveOccurred())

			beforeJSON, err := json.Marshal(desiredBefore)
			Expect(err).NotTo(HaveOccurred())
			afterJSON, err := json.Marshal(desiredAfter)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(afterJSON)).To(Equal(string(beforeJSON)))

			// A successful derive clears the memory, so the same message is logged again.
			s.TestUpdateUserSpec(config.UserSpec{Config: "valid"})
			Expect(s.TestTick(ctx)).NotTo(HaveOccurred())

			s.TestUpdateUserSpec(config.UserSpec{Config: "{{ misspelled name"})
			for i := 0; i < 3; i++ {
				Expect(s.TestTick(ctx)).To(HaveOccurred())
			}

			Expect(recorder.infoCount("config_error")).To(Equal(2))
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
			Expect(recorder.infoCount("config_error")).To(Equal(0))
		})
	})

	Describe("A child whose spec has an unknown state value", func() {
		// The value is the user's typo, so misspell must not flag it.
		const brokenStateConfig = "state: runing\nname: b1" //nolint:misspell

		It("logs once with a config error naming the value and stays out of Sentry", func() {
			recorder := newRecordingSentryLogger()

			err := factory.RegisterFactoryByType("statetypochild",
				func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &stateTypoWorker{}
				})
			Expect(err).NotTo(HaveOccurred())

			err = factory.RegisterSupervisorFactoryByType("statetypochild", func(cfg any) any {
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
			}, &childSpecParentWorker{childWorkerType: "statetypochild"})
			Expect(err).NotTo(HaveOccurred())

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: brokenStateConfig})

			for range 3 {
				Expect(parentSuper.TestTick(ctx)).NotTo(HaveOccurred())
			}

			Expect(recorder.infoCount("config_error")).To(Equal(1))

			childLogFields := logFieldMap(recorder.infoCalls("config_error")[0])
			Expect(childLogFields["child_name"]).To(Equal("cfgerr-child"))
			Expect(childLogFields["field"]).To(Equal("state"),
				"the child log line names the config error's field")
			Expect(fmt.Sprint(childLogFields["error"])).To(ContainSubstring("runing"), //nolint:misspell
				"the child log line carries the underlying error")
			Expect(childLogFields["hierarchy_path"]).To(ContainSubstring("parent-worker"),
				"the child log line names the parent whose child failed")

			Expect(parentSuper.GetChildren()).To(BeEmpty(),
				"a worker with a config mistake must not start")

			Expect(recorder.sentryErrorEvents()).To(BeEmpty())
			Expect(recorder.sentryWarnEvents()).To(BeEmpty())

			Expect(recorder.infoCalls("config_error")[0].Fields).To(ContainElement(
				WithTransform(func(f deps.Field) any { return f.Value }, ContainSubstring("runing"))), //nolint:misspell
				"the log line must name the rejected state value")
		})
	})
})
