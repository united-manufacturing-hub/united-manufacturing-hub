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

// Tests for WorkerBase[TConfig, TStatus, TDeps]: constructor invariants, config parsing,
// dependency binding, and capability interface enforcement.
// Status-wrapping tests are added in later lifecycle layers.

package fsmv2_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// wbTestConfig embeds BaseUserSpec so lifecycle state propagation is exercised.
type wbTestConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	URL                 string `yaml:"url"`
	Port                int    `yaml:"port"`
}

// wbTestStatus is the observed status for the test worker. It is referenced
// only via WorkerBase generic instantiation; the blank-var below keeps the
// unused linter (which doesn't trace through type parameters) satisfied.
type wbTestStatus struct {
	Health string
}

var _ = wbTestStatus{}

// wbTestDeps is the dependency set for the test worker.
type wbTestDeps struct {
	Value int
}

// wbTestStateReader satisfies deps.StateReader with a no-op.
type wbTestStateReader struct{}

func (r *wbTestStateReader) LoadObservedTyped(_ context.Context, _ string, _ string, _ interface{}) error {
	return nil
}

// TestWorker embeds WorkerBase the same way a production worker would.
// It exposes CollectObservedState as a no-op to satisfy the Worker interface.
type TestWorker struct {
	fsmv2.WorkerBase[wbTestConfig, wbTestStatus, *wbTestDeps]
}

// CollectObservedState satisfies the Worker interface (no-op for testing).
func (w *TestWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return nil, nil
}

// Compile-time check: TestWorker satisfies DependencyProvider via the embedded WorkerBase.
var _ fsmv2.DependencyProvider = &TestWorker{}

// Compile-time check: TestWorker does NOT implement capability interfaces.
// These are intentionally invalid — they must fail if uncommented:
//   var _ fsmv2.ActionProvider        = &TestWorker{} // must NOT compile
//   var _ fsmv2.MetricsProvider       = &TestWorker{} // must NOT compile
//   var _ fsmv2.GracefulShutdowner    = &TestWorker{} // must NOT compile
//   var _ fsmv2.ChildrenViewConsumer  = &TestWorker{} // must NOT compile

// wbMakeIdentity creates a test Identity with the given worker type.
func wbMakeIdentity(workerType string) deps.Identity {
	return deps.Identity{
		ID:         "test-id-001",
		Name:       "test-worker",
		WorkerType: workerType,
	}
}

// wbMakeStateReader returns a no-op StateReader for tests.
func wbMakeStateReader() deps.StateReader {
	return &wbTestStateReader{}
}

var _ = Describe("WorkerBase", func() {
	var (
		id     deps.Identity
		logger deps.FSMLogger
		sr     deps.StateReader
		worker *TestWorker
	)

	BeforeEach(func() {
		id = wbMakeIdentity("test-worker-type")
		logger = deps.NewNopFSMLogger()
		sr = wbMakeStateReader()
		worker = &TestWorker{}
	})

	Describe("InitBase", func() {
		It("returns a non-nil BaseDependencies pointer", func() {
			bd := worker.InitBase(id, logger, sr)
			Expect(bd).NotTo(BeNil())
		})

		It("returned BaseDependencies carries the correct worker type", func() {
			bd := worker.InitBase(id, logger, sr)
			Expect(bd.GetWorkerType()).To(Equal(id.WorkerType))
		})

		It("returned BaseDependencies carries the correct worker ID", func() {
			bd := worker.InitBase(id, logger, sr)
			Expect(bd.GetWorkerID()).To(Equal(id.ID))
		})

		It("returned BaseDependencies carries the correct logger", func() {
			bd := worker.InitBase(id, logger, sr)
			Expect(bd.GetLogger()).To(Equal(logger))
		})

		It("sets identity accessible via Identity()", func() {
			worker.InitBase(id, logger, sr)
			Expect(worker.Identity()).To(Equal(id))
		})

		It("sets logger accessible via Logger()", func() {
			worker.InitBase(id, logger, sr)
			Expect(worker.Logger()).To(Equal(logger))
		})

		It("marks initialized without yet setting ConfigReady", func() {
			worker.InitBase(id, logger, sr)
			Expect(worker.ConfigReady()).To(BeFalse())
		})
	})

	Describe("Config", func() {
		It("returns zero-value before first DeriveDesiredState", func() {
			worker.InitBase(id, logger, sr)
			cfg := worker.Config()
			Expect(cfg.URL).To(Equal(""))
			Expect(cfg.Port).To(Equal(0))
		})

		It("returns parsed config after DeriveDesiredState with valid spec", func() {
			worker.InitBase(id, logger, sr)
			spec := config.UserSpec{
				Config: "url: http://example.com\nport: 9090",
			}
			_, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			cfg := worker.Config()
			Expect(cfg.URL).To(Equal("http://example.com"))
			Expect(cfg.Port).To(Equal(9090))
		})
	})

	Describe("ConfigReady", func() {
		It("returns false before DeriveDesiredState", func() {
			worker.InitBase(id, logger, sr)
			Expect(worker.ConfigReady()).To(BeFalse())
		})

		It("returns true after DeriveDesiredState with nil spec", func() {
			worker.InitBase(id, logger, sr)
			_, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(worker.ConfigReady()).To(BeTrue())
		})

		It("returns true after DeriveDesiredState with valid UserSpec", func() {
			worker.InitBase(id, logger, sr)
			spec := config.UserSpec{Config: "url: http://localhost\nport: 8080"}
			_, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(worker.ConfigReady()).To(BeTrue())
		})
	})

	Describe("Identity", func() {
		It("returns the identity set by InitBase", func() {
			worker.InitBase(id, logger, sr)
			got := worker.Identity()
			Expect(got.ID).To(Equal("test-id-001"))
			Expect(got.WorkerType).To(Equal("test-worker-type"))
			Expect(got.Name).To(Equal("test-worker"))
		})

		It("returns zero value if InitBase not called", func() {
			got := worker.Identity()
			Expect(got.ID).To(Equal(""))
			Expect(got.WorkerType).To(Equal(""))
		})
	})

	Describe("Logger", func() {
		It("returns the logger set by InitBase", func() {
			worker.InitBase(id, logger, sr)
			Expect(worker.Logger()).To(Equal(logger))
		})
	})

	Describe("BindDeps / GetDependenciesAny", func() {
		It("round-trip: bound value is returned as any", func() {
			worker.InitBase(id, logger, sr)
			d := &wbTestDeps{Value: 42}
			worker.BindDeps(d)
			got := worker.GetDependenciesAny()
			Expect(got).To(Equal(d))
		})

		It("returns nil any before BindDeps", func() {
			worker.InitBase(id, logger, sr)
			got := worker.GetDependenciesAny()
			Expect(got).To(BeNil())
		})

		It("returns updated value after second BindDeps call", func() {
			worker.InitBase(id, logger, sr)
			first := &wbTestDeps{Value: 1}
			second := &wbTestDeps{Value: 2}
			worker.BindDeps(first)
			worker.BindDeps(second)
			got := worker.GetDependenciesAny()
			Expect(got).To(Equal(second))
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			worker.InitBase(id, logger, sr)
		})

		It("nil spec returns WrappedDesiredState with zero TConfig", func() {
			ds, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTestConfig])
			Expect(ok).To(BeTrue())
			Expect(wds.Config.URL).To(Equal(""))
			Expect(wds.Config.Port).To(Equal(0))
		})

		It("nil spec sets ConfigReady to true", func() {
			_, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(worker.ConfigReady()).To(BeTrue())
		})

		It("valid UserSpec with YAML parses URL and Port", func() {
			spec := config.UserSpec{Config: "url: http://example.com\nport: 9090"}
			ds, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTestConfig])
			Expect(ok).To(BeTrue())
			Expect(wds.Config.URL).To(Equal("http://example.com"))
			Expect(wds.Config.Port).To(Equal(9090))
		})

		It("valid UserSpec with state=stopped is accepted and parses config", func() {
			spec := config.UserSpec{Config: "state: stopped\nurl: http://example.com\nport: 1234"}
			ds, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTestConfig])
			Expect(ok).To(BeTrue())
			Expect(wds.Config.URL).To(Equal("http://example.com"))
			Expect(wds.Config.Port).To(Equal(1234))
		})

		It("valid UserSpec with state=running is accepted and parses config", func() {
			spec := config.UserSpec{Config: "state: running\nurl: http://example.com\nport: 5678"}
			ds, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTestConfig])
			Expect(ok).To(BeTrue())
			Expect(wds.Config.URL).To(Equal("http://example.com"))
			Expect(wds.Config.Port).To(Equal(5678))
		})

		It("invalid spec type (non-UserSpec) returns error", func() {
			_, err := worker.DeriveDesiredState("this-is-not-a-UserSpec")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected config.UserSpec"))
		})

		It("YAML unmarshal error returns error", func() {
			// Provide invalid YAML that fails unmarshal into wbTestConfig.
			// Port expects int, so a non-numeric value triggers an error.
			spec := config.UserSpec{Config: "port: [not, an, int]"}
			_, err := worker.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())

			var cfgErr *config.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("cannot unmarshal"))
		})

		It("invalid desired state (e.g. 'invalid_state') returns error", func() {
			spec := config.UserSpec{Config: "state: invalid_state"}
			_, err := worker.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid desired state"))
		})

		It("an unknown state value is a config error naming the field", func() {
			// The value is the user's typo, so misspell must not flag it.
			spec := config.UserSpec{Config: "state: runing"} //nolint:misspell
			_, err := worker.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())

			var cfgErr *config.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue(),
				"a state typo is the user's mistake, so it must be a *config.ConfigError, got %T", err)
			Expect(cfgErr.Field).To(Equal("state"))
			Expect(err.Error()).To(ContainSubstring("runing")) //nolint:misspell
		})

		It("nil spec returns a default WrappedDesiredState with empty config", func() {
			ds, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTestConfig])
			Expect(ok).To(BeTrue())
			Expect(wds.Config).To(Equal(wbTestConfig{}))
			Expect(wds.IsShutdownRequested()).To(BeFalse())
		})

		It("config is cached: Config() matches after DeriveDesiredState", func() {
			spec := config.UserSpec{Config: "url: http://cached.example.com\nport: 7070"}
			_, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			cached := worker.Config()
			Expect(cached.URL).To(Equal("http://cached.example.com"))
			Expect(cached.Port).To(Equal(7070))
		})
	})

	Describe("GetInitialState", func() {
		It("panics if InitBase not called", func() {
			uninitWorker := &TestWorker{}
			Expect(func() {
				uninitWorker.GetInitialState()
			}).To(PanicWith(ContainSubstring("InitBase was not called")))
		})

		It("panics if no state registered for worker type", func() {
			unregisteredID := wbMakeIdentity("unregistered-worker-type-xyz")
			worker.InitBase(unregisteredID, logger, sr)
			Expect(func() {
				worker.GetInitialState()
			}).To(PanicWith(ContainSubstring("no initial state registered for worker type")))
		})

		It("returns registered state for the worker type", func() {
			const testWorkerType = "wb-test-registered-type"
			mockState := &wbMockState{}
			fsmv2.RegisterInitialState(testWorkerType, mockState)
			defer fsmv2.ResetInitialStateRegistry()

			registeredID := wbMakeIdentity(testWorkerType)
			regWorker := &TestWorker{}
			regWorker.InitBase(registeredID, logger, sr)

			got := regWorker.GetInitialState()
			Expect(got).To(Equal(mockState))
		})
	})

	Describe("L5 capability interface invariants", func() {
		It("satisfies DependencyProvider", func() {
			// Compile-time check at top of file; this verifies it at runtime too.
			var _ fsmv2.DependencyProvider = &TestWorker{}
		})

		It("does not implement ActionProvider", func() {
			var w interface{} = &TestWorker{}
			_, ok := w.(fsmv2.ActionProvider)
			Expect(ok).To(BeFalse())
		})

		It("does not implement MetricsProvider", func() {
			var w interface{} = &TestWorker{}
			_, ok := w.(fsmv2.MetricsProvider)
			Expect(ok).To(BeFalse())
		})

		It("does not implement GracefulShutdowner", func() {
			var w interface{} = &TestWorker{}
			_, ok := w.(fsmv2.GracefulShutdowner)
			Expect(ok).To(BeFalse())
		})

		It("does not implement ChildrenViewConsumer", func() {
			var w interface{} = &TestWorker{}
			_, ok := w.(fsmv2.ChildrenViewConsumer)
			Expect(ok).To(BeFalse())
		})
	})

	Describe("WrappedDesiredState via DeriveDesiredState", func() {
		BeforeEach(func() {
			worker.InitBase(id, logger, sr)
		})

		It("returned WrappedDesiredState has correct Config", func() {
			spec := config.UserSpec{Config: "url: http://wrapped.example.com\nport: 3030"}
			ds, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTestConfig])
			Expect(ok).To(BeTrue())
			Expect(wds.Config.URL).To(Equal("http://wrapped.example.com"))
			Expect(wds.Config.Port).To(Equal(3030))
		})

		It("returned WrappedDesiredState satisfies DesiredState interface", func() {
			ds, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			var _ = ds
			Expect(ds.IsShutdownRequested()).To(BeFalse())
		})

		It("returned WrappedDesiredState satisfies ShutdownRequestable interface", func() {
			ds, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			sr, ok := ds.(fsmv2.ShutdownRequestable)
			Expect(ok).To(BeTrue())
			sr.SetShutdownRequested(true)
			Expect(ds.IsShutdownRequested()).To(BeTrue())
		})
	})
})

// NoDepsWorker embeds WorkerBase with struct{} as TDeps, so its GetDependenciesAny
// returns a non-nil interface boxing struct{}{}. Returning nil instead changes
// nothing: the collector attaches framework metrics and action history from its
// own locals before the deps guard, so a no-deps worker has no reason to override.
type NoDepsWorker struct {
	fsmv2.WorkerBase[wbTestConfig, wbTestStatus, struct{}]
}

func (w *NoDepsWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return nil, nil
}

var _ = Describe("WorkerBase no-deps footgun", func() {
	Describe("GetDependenciesAny with struct{} TDeps", func() {
		It("returns non-nil any wrapping struct{}{} even without BindDeps", func() {
			// WorkerBase[..., struct{}] zero-initialises typedDeps to struct{}{}.
			// GetDependenciesAny boxes struct{}{} into a non-nil interface value.
			// This is inert now: the collector attaches framework metrics and action
			// history from its own locals before the deps guard, so a worker no
			// longer needs a nil override to opt out of injection.
			id := wbMakeIdentity("noop-worker-type")
			logger := deps.NewNopFSMLogger()
			sr := wbMakeStateReader()

			w := &NoDepsWorker{}
			w.InitBase(id, logger, sr)

			got := w.GetDependenciesAny()
			Expect(got).NotTo(BeNil(),
				"struct{}{} boxes to a non-nil interface value")
		})
	})
})

// wbMockState is a minimal State[any, any] implementation for registry tests.
type wbMockState struct{}

func (s *wbMockState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.NextResult[any, any]{State: s, Reason: "mock"}
}

func (s *wbMockState) String() string { return "wb_mock_state" }

func (s *wbMockState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopped
}

// wbTplConfig pairs a Template field with a plain field, so tests can tell the
// rendered field from the field the loader must reject.
type wbTplConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	Name                string          `yaml:"name"`
	Read                config.Template `yaml:"read"`
}

// TplTestWorker embeds WorkerBase the same way a production worker with a
// Template field would.
type TplTestWorker struct {
	fsmv2.WorkerBase[wbTplConfig, wbTestStatus, *wbTestDeps]
}

// CollectObservedState satisfies the Worker interface (no-op for testing).
func (w *TplTestWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return nil, nil
}

var _ = Describe("WorkerBase template fields", func() {
	var (
		id     deps.Identity
		logger deps.FSMLogger
		sr     deps.StateReader
		worker *TplTestWorker
	)

	BeforeEach(func() {
		id = wbMakeIdentity("tpl-test-worker-type")
		logger = deps.NewNopFSMLogger()
		sr = wbMakeStateReader()
		worker = &TplTestWorker{}
		worker.InitBase(id, logger, sr)
	})

	It("renders a Template field against the config's own fields", func() {
		spec := config.UserSpec{Config: "name: b1\nread: \"{{ .name }}-x\""}

		ds, err := worker.DeriveDesiredState(spec)

		Expect(err).NotTo(HaveOccurred())
		wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTplConfig])
		Expect(ok).To(BeTrue())
		Expect(wds.Config.Name).To(Equal("b1"))
		Expect(wds.Config.Read).To(Equal(config.Template("b1-x")))
	})

	It("renders a Template field against the entry's own variables section", func() {
		entry := map[string]any{
			"variables": map[string]any{"X": "a"},
			"read":      "{{ .variables.X }}",
		}

		spec, err := config.NewChildSpec[map[string]any]("b1", "tpl-test-worker-type", entry, true)
		Expect(err).NotTo(HaveOccurred())

		ds, err := worker.DeriveDesiredState(spec.UserSpec)

		Expect(err).NotTo(HaveOccurred())

		wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTplConfig])
		Expect(ok).To(BeTrue())
		Expect(wds.Config.Read).To(Equal(config.Template("a")))
	})

	It("rejects an entry variables section when the spec also carries a variables bundle", func() {
		spec, err := config.NewChildSpec[map[string]any]("b1", "tpl-test-worker-type", map[string]any{
			"variables": map[string]any{"X": "a"},
			"read":      "{{ .name }}",
		}, true)
		Expect(err).NotTo(HaveOccurred())

		userSpec := spec.UserSpec
		userSpec.Variables = map[string]any{"X": "b"}

		_, err = worker.DeriveDesiredState(userSpec)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("variables"))
	})

	It("wraps a render error in a ConfigError naming the field", func() {
		spec := config.UserSpec{Config: "read: \"{{ .nope }}\""}

		_, err := worker.DeriveDesiredState(spec)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(HavePrefix("failed to render the template fields"))
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("read"))
	})

	It("rejects a placeholder in a plain field", func() {
		spec := config.UserSpec{Config: "name: \"{{ .x }}\""}

		_, err := worker.DeriveDesiredState(spec)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("name"))
	})

	It("rejects a placeholder under a config key that no field declares", func() {
		spec := config.UserSpec{Config: "name: b1\nfoo: \"{{ .IP }}\"\nread: \"{{ .name }}\""}

		_, err := worker.DeriveDesiredState(spec)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("foo"))
	})

	It("keeps a config key that no field declares when it carries no placeholder", func() {
		spec := config.UserSpec{Config: "name: b1\nfoo: bar\nread: \"{{ .foo }}\""}

		ds, err := worker.DeriveDesiredState(spec)

		Expect(err).NotTo(HaveOccurred())

		wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTplConfig])
		Expect(ok).To(BeTrue())

		Expect(wds.Config.Read).To(Equal(config.Template("bar")),
			"the undeclared key must survive into the names, so a template can use it")
	})

	It("renders with the Names a parent hands down, and with ConfigValues when it hands down none", func() {
		namesSpec := config.UserSpec{
			Config: `read: "{{ .x }}"`,
			Names:  map[string]any{"x": "from-parent"},
		}

		ds, err := worker.DeriveDesiredState(namesSpec)

		Expect(err).NotTo(HaveOccurred())
		wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTplConfig])
		Expect(ok).To(BeTrue())
		Expect(wds.Config.Read).To(Equal(config.Template("from-parent")))

		valuesSpec := config.UserSpec{
			Config:       `read: "{{ .config.historian.timescale.host }}"`,
			ConfigValues: map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}},
		}

		ds, err = worker.DeriveDesiredState(valuesSpec)

		Expect(err).NotTo(HaveOccurred())
		wds, ok = ds.(*fsmv2.WrappedDesiredState[wbTplConfig])
		Expect(ok).To(BeTrue())
		Expect(wds.Config.Read).To(Equal(config.Template("db")))
	})

	It("builds the names when a parent hands down an empty Names map", func() {
		spec := config.UserSpec{
			Config: "name: b1\nread: \"{{ .name }}-x\"",
			Names:  map[string]any{},
		}

		ds, err := worker.DeriveDesiredState(spec)

		Expect(err).NotTo(HaveOccurred())
		wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTplConfig])
		Expect(ok).To(BeTrue())
		Expect(wds.Config.Read).To(Equal(config.Template("b1-x")))
	})

	It("renders a variable through the variables section", func() {
		spec := config.UserSpec{
			Config:    `read: "{{ .variables.A }}"`,
			Variables: map[string]any{"A": "1"},
		}

		ds, err := worker.DeriveDesiredState(spec)

		Expect(err).NotTo(HaveOccurred())
		wds, ok := ds.(*fsmv2.WrappedDesiredState[wbTplConfig])
		Expect(ok).To(BeTrue())
		Expect(wds.Config.Read).To(Equal(config.Template("1")))
	})

	It("errors naming the top-level name when the worker declares no shortcut for it", func() {
		// wbTplConfig declares no shortcuts, so {{ .A }} has no top-level name.
		spec := config.UserSpec{
			Config:    `read: "{{ .variables.A }}-{{ .A }}"`,
			Variables: map[string]any{"A": "1"},
		}

		_, err := worker.DeriveDesiredState(spec)

		Expect(err).To(HaveOccurred())
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("read"))
		Expect(err.Error()).To(ContainSubstring(`"A"`),
			"the render error must name the top-level name the worker declares no shortcut for")
	})
})

// wbHookConfig is a config type with a Template field and both optional name
// methods. ReadyMade publishes the ready-from config value a parent hands down
// under the name ready, and Shortcuts publishes the short-from value under the
// name short.
type wbHookConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	Read                config.Template `yaml:"read"`
}

func (c wbHookConfig) ReadyMade(cfgValues map[string]any) (map[string]any, error) {
	return map[string]any{"ready": cfgValues["ready-from"]}, nil
}

func (c wbHookConfig) Shortcuts(cfgValues map[string]any) (map[string]any, error) {
	return map[string]any{"short": cfgValues["short-from"]}, nil
}

// HookTestWorker embeds WorkerBase with a config type that implements the
// optional name methods.
type HookTestWorker struct {
	fsmv2.WorkerBase[wbHookConfig, wbTestStatus, *wbTestDeps]
}

func (w *HookTestWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return nil, nil
}

var _ = Describe("WorkerBase config-type name methods", func() {
	It("renders the value ReadyMade returns and the value Shortcuts returns", func() {
		worker := &HookTestWorker{}
		worker.InitBase(
			wbMakeIdentity("hook-test-worker-type"),
			deps.NewNopFSMLogger(),
			wbMakeStateReader(),
		)

		spec := config.UserSpec{
			Config: `read: "{{ .ready }} {{ .short }}"`,
			ConfigValues: map[string]any{
				"ready-from": "ready-value",
				"short-from": "short-value",
			},
		}

		ds, err := worker.DeriveDesiredState(spec)

		Expect(err).NotTo(HaveOccurred())
		wds, ok := ds.(*fsmv2.WrappedDesiredState[wbHookConfig])
		Expect(ok).To(BeTrue())
		Expect(wds.Config.Read).To(Equal(config.Template("ready-value short-value")))
	})
})
