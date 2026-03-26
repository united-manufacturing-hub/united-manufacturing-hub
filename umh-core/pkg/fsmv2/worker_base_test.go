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

package fsmv2_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// workerTestConfig is a typed config used in WorkerBase tests.
type workerTestConfig struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
}

// stateGetterConfig embeds BaseUserSpec so it implements config.StateGetter.
type stateGetterConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	Host                string `json:"host" yaml:"host"`
}

// workerTestStatus is a typed status used in WorkerBase tests.
type workerTestStatus struct {
	Reachable bool  `json:"reachable"`
	LatencyMs int64 `json:"latencyMs"`
}

var _ = Describe("WorkerBase", func() {
	var (
		wb       *fsmv2.WorkerBase[workerTestConfig, workerTestStatus]
		identity deps.Identity
	)

	BeforeEach(func() {
		wb = &fsmv2.WorkerBase[workerTestConfig, workerTestStatus]{}
		identity = deps.Identity{
			ID:         "test-worker-1",
			Name:       "test-worker",
			WorkerType: "test",
		}
	})

	Describe("InitBase", func() {
		It("initializes the worker with identity, logger, and state reader", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)
			Expect(wb.Identity()).To(Equal(identity))
			Expect(wb.Logger()).NotTo(BeNil())
		})
	})

	Describe("WrapStatus", func() {
		It("returns Observation with CollectedAt set and correct Status", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)
			before := time.Now()
			status := workerTestStatus{Reachable: true, LatencyMs: 42}

			obs := wb.WrapStatus(status)

			Expect(obs).NotTo(BeNil())
			Expect(obs.GetTimestamp()).To(BeTemporally(">=", before))
			Expect(obs.GetTimestamp()).To(BeTemporally("<=", time.Now()))

			typed, ok := obs.(fsmv2.Observation[workerTestStatus])
			Expect(ok).To(BeTrue(), "WrapStatus should return Observation[TStatus]")
			Expect(typed.Status.Reachable).To(BeTrue())
			Expect(typed.Status.LatencyMs).To(Equal(int64(42)))
		})

		It("handles uninitialized WorkerBase gracefully (no panic)", func() {
			uninit := &fsmv2.WorkerBase[workerTestConfig, workerTestStatus]{}
			status := workerTestStatus{Reachable: false}

			Expect(func() {
				obs := uninit.WrapStatus(status)
				Expect(obs).NotTo(BeNil())
			}).NotTo(Panic())
		})

		It("copies framework metrics from baseDeps when available", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)

			status := workerTestStatus{Reachable: true}
			obs := wb.WrapStatus(status)

			typed, ok := obs.(fsmv2.Observation[workerTestStatus])
			Expect(ok).To(BeTrue())
			// Framework metrics are zero since supervisor hasn't injected them,
			// but the field should exist (not panic).
			Expect(typed.Metrics.Framework.StateTransitionsTotal).To(Equal(int64(0)))
		})

		It("drains worker metrics from MetricsRecorder", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)

			// Record some metrics via baseDeps
			bd := wb.GetDependenciesAny().(*deps.BaseDependencies)
			bd.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 5)
			bd.MetricsRecorder().SetGauge(deps.GaugeConsecutiveErrors, 3.0)

			status := workerTestStatus{Reachable: true}
			obs := wb.WrapStatus(status)

			typed, ok := obs.(fsmv2.Observation[workerTestStatus])
			Expect(ok).To(BeTrue())
			Expect(typed.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(5)))
			Expect(typed.Metrics.Worker.Gauges["consecutive_errors"]).To(Equal(3.0))

			// Second WrapStatus should have empty worker metrics (drained)
			obs2 := wb.WrapStatus(status)
			typed2 := obs2.(fsmv2.Observation[workerTestStatus])
			Expect(typed2.Metrics.Worker.Counters).To(BeEmpty())
			Expect(typed2.Metrics.Worker.Gauges).To(BeEmpty())
		})
	})

	Describe("InitBase returns BaseDeps", func() {
		It("returns the same BaseDependencies that WrapStatus reads from", func() {
			bd := wb.InitBase(identity, mockLogger, mockStateReader)
			Expect(bd).NotTo(BeNil())

			bd.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 7)

			obs := wb.WrapStatus(workerTestStatus{Reachable: true})
			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(7)))
		})

		It("documents that a separate BaseDependencies is invisible to WrapStatus", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)

			separate := deps.NewBaseDependencies(mockLogger, mockStateReader, identity)
			separate.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 99)

			obs := wb.WrapStatus(workerTestStatus{Reachable: true})
			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Metrics.Worker.Counters).To(BeEmpty(),
				"metrics on a separate BaseDependencies must not appear in WrapStatus")
		})
	})

	Describe("Config and ConfigReady", func() {
		It("returns zero-value Config before first DeriveDesiredState", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)
			cfg := wb.Config()
			Expect(cfg.Host).To(BeEmpty())
			Expect(cfg.Port).To(Equal(0))
		})

		It("returns false for ConfigReady before first DeriveDesiredState", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)
			Expect(wb.ConfigReady()).To(BeFalse())
		})

		It("returns correct Config and true ConfigReady after DeriveDesiredState", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)

			spec := config.UserSpec{
				Config: `host: "localhost"
port: 8080`,
			}
			_, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(wb.ConfigReady()).To(BeTrue())
			cfg := wb.Config()
			Expect(cfg.Host).To(Equal("localhost"))
			Expect(cfg.Port).To(Equal(8080))
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			wb.InitBase(identity, mockLogger, mockStateReader)
		})

		It("returns default WrappedDesiredState with running state for nil spec", func() {
			ds, err := wb.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ds).NotTo(BeNil())
			Expect(ds.GetState()).To(Equal("running"))
			Expect(ds.IsShutdownRequested()).To(BeFalse())
		})

		It("sets configReady=true even for nil spec", func() {
			Expect(wb.ConfigReady()).To(BeFalse())
			_, err := wb.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(wb.ConfigReady()).To(BeTrue())
		})

		It("parses UserSpec YAML into typed config", func() {
			spec := config.UserSpec{
				Config: `host: "192.168.1.100"
port: 502`,
			}
			ds, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			typed, ok := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(ok).To(BeTrue(), "should return *WrappedDesiredState[TConfig]")
			Expect(typed.Config.Host).To(Equal("192.168.1.100"))
			Expect(typed.Config.Port).To(Equal(502))
		})

		It("renders templates before parsing", func() {
			spec := config.UserSpec{
				Config: `host: "{{ .IP }}"
port: {{ .PORT }}`,
				Variables: config.VariableBundle{
					User: map[string]interface{}{
						"IP":   "10.0.0.1",
						"PORT": 1234,
					},
				},
			}
			ds, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			typed := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(typed.Config.Host).To(Equal("10.0.0.1"))
			Expect(typed.Config.Port).To(Equal(1234))
		})

		It("returns error for invalid spec type", func() {
			_, err := wb.DeriveDesiredState("not-a-userspec")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected UserSpec"))
		})

		It("returns error for invalid template syntax", func() {
			spec := config.UserSpec{
				Config: `host: "{{ .Missing }}"`,
			}
			_, err := wb.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template"))
		})

		It("returns error for invalid YAML in config", func() {
			spec := config.UserSpec{
				Config: `[invalid yaml`,
			}
			_, err := wb.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())
		})

		It("extracts state from config implementing StateGetter", func() {
			sgWb := &fsmv2.WorkerBase[stateGetterConfig, workerTestStatus]{}
			sgWb.InitBase(identity, mockLogger, mockStateReader)

			spec := config.UserSpec{
				Config: `state: "stopped"
host: "example.com"`,
			}
			ds, err := sgWb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.GetState()).To(Equal("stopped"))

			typed := ds.(*fsmv2.WrappedDesiredState[stateGetterConfig])
			Expect(typed.Config.Host).To(Equal("example.com"))
		})

		It("caches config for subsequent Config() calls", func() {
			spec := config.UserSpec{Config: `host: "first"
port: 1`}
			_, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(wb.Config().Host).To(Equal("first"))

			spec2 := config.UserSpec{Config: `host: "second"
port: 2`}
			_, err = wb.DeriveDesiredState(spec2)
			Expect(err).NotTo(HaveOccurred())
			Expect(wb.Config().Host).To(Equal("second"))
		})

		It("calls postParseHook after config parsing", func() {
			wb.SetPostParseHook(func(cfg *workerTestConfig) error {
				cfg.Host = cfg.Host + "-modified"
				return nil
			})

			spec := config.UserSpec{Config: `host: "original"
port: 8080`}
			ds, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			typed := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(typed.Config.Host).To(Equal("original-modified"))
			Expect(wb.Config().Host).To(Equal("original-modified"))
		})

		It("returns error when postParseHook fails", func() {
			wb.SetPostParseHook(func(_ *workerTestConfig) error {
				return fmt.Errorf("validation failed")
			})

			spec := config.UserSpec{Config: `host: "test"
port: 1`}
			_, err := wb.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validation failed"))
		})

		It("calls postParseHook on nil-spec path", func() {
			var hookCalled bool
			wb.SetPostParseHook(func(cfg *workerTestConfig) error {
				hookCalled = true
				cfg.Host = "default-host"
				return nil
			})

			ds, err := wb.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(hookCalled).To(BeTrue())

			typed := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(typed.Config.Host).To(Equal("default-host"))
		})

		It("populates ChildrenSpecs from childSpecsFactory", func() {
			wb.SetChildSpecsFactory(func(cfg workerTestConfig, _ config.UserSpec) []config.ChildSpec {
				return []config.ChildSpec{
					{Name: "child-" + cfg.Host, WorkerType: "child"},
				}
			})

			spec := config.UserSpec{Config: `host: "parent"
port: 1`}
			ds, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			typed := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(typed.ChildrenSpecs).To(HaveLen(1))
			Expect(typed.ChildrenSpecs[0].Name).To(Equal("child-parent"))
		})

		It("populates ChildrenSpecs on nil-spec path", func() {
			wb.SetChildSpecsFactory(func(_ workerTestConfig, _ config.UserSpec) []config.ChildSpec {
				return []config.ChildSpec{
					{Name: "default-child", WorkerType: "child"},
				}
			})

			ds, err := wb.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())

			typed := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(typed.ChildrenSpecs).To(HaveLen(1))
			Expect(typed.ChildrenSpecs[0].Name).To(Equal("default-child"))
		})

		It("passes raw UserSpec to childSpecsFactory for child DDS", func() {
			var receivedSpec config.UserSpec
			wb.SetChildSpecsFactory(func(_ workerTestConfig, spec config.UserSpec) []config.ChildSpec {
				receivedSpec = spec
				return []config.ChildSpec{
					{Name: "transport", WorkerType: "transport", UserSpec: spec},
				}
			})

			spec := config.UserSpec{
				Config: `host: "parent"
port: 1`,
				Variables: config.VariableBundle{
					User: map[string]interface{}{"KEY": "val"},
				},
			}
			ds, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedSpec.Config).To(Equal(spec.Config))
			Expect(receivedSpec.Variables.User).To(HaveKeyWithValue("KEY", "val"))

			typed := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(typed.ChildrenSpecs[0].UserSpec.Config).To(Equal(spec.Config))
		})

		It("passes empty UserSpec to childSpecsFactory on nil-spec path", func() {
			var receivedSpec config.UserSpec
			wb.SetChildSpecsFactory(func(_ workerTestConfig, spec config.UserSpec) []config.ChildSpec {
				receivedSpec = spec
				return nil
			})

			_, err := wb.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedSpec.Config).To(BeEmpty())
		})

		It("calls postParseHook before childSpecsFactory", func() {
			wb.SetPostParseHook(func(cfg *workerTestConfig) error {
				cfg.Port = 9999
				return nil
			})
			wb.SetChildSpecsFactory(func(cfg workerTestConfig, _ config.UserSpec) []config.ChildSpec {
				return []config.ChildSpec{
					{Name: fmt.Sprintf("child-%d", cfg.Port), WorkerType: "child"},
				}
			})

			spec := config.UserSpec{Config: `host: "test"
port: 1`}
			ds, err := wb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			typed := ds.(*fsmv2.WrappedDesiredState[workerTestConfig])
			Expect(typed.ChildrenSpecs[0].Name).To(Equal("child-9999"))
		})
	})

	Describe("WrapStatusAccumulated", func() {
		It("accumulates counters additively with previous state", func() {
			prevObs := fsmv2.Observation[workerTestStatus]{
				CollectedAt: time.Now().Add(-time.Second),
				Status:      workerTestStatus{Reachable: true},
			}
			prevObs.Metrics.Worker = deps.Metrics{
				Counters: map[string]int64{"pull_ops": 10},
				Gauges:   map[string]float64{},
			}

			sr := &configurableStateReader{previousState: prevObs}
			wb.InitBase(identity, mockLogger, sr)

			bd := wb.GetDependenciesAny().(*deps.BaseDependencies)
			bd.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 5)

			obs := wb.WrapStatusAccumulated(context.Background(), workerTestStatus{Reachable: true})

			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(15)))
		})

		It("replaces gauges from drain, keeps previous gauges not in drain", func() {
			prevObs := fsmv2.Observation[workerTestStatus]{
				CollectedAt: time.Now().Add(-time.Second),
				Status:      workerTestStatus{},
			}
			prevObs.Metrics.Worker = deps.Metrics{
				Counters: map[string]int64{},
				Gauges: map[string]float64{
					"consecutive_errors":   5.0,
					"last_pull_latency_ms": 100.0,
				},
			}

			sr := &configurableStateReader{previousState: prevObs}
			wb.InitBase(identity, mockLogger, sr)

			bd := wb.GetDependenciesAny().(*deps.BaseDependencies)
			bd.MetricsRecorder().SetGauge(deps.GaugeConsecutiveErrors, 3.0)

			obs := wb.WrapStatusAccumulated(context.Background(), workerTestStatus{})

			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Metrics.Worker.Gauges["consecutive_errors"]).To(Equal(3.0))
			Expect(typed.Metrics.Worker.Gauges["last_pull_latency_ms"]).To(Equal(100.0))
		})

		It("starts fresh when CSE read fails (no previous state)", func() {
			sr := &configurableStateReader{err: fmt.Errorf("no state")}
			wb.InitBase(identity, mockLogger, sr)

			bd := wb.GetDependenciesAny().(*deps.BaseDependencies)
			bd.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 5)
			bd.MetricsRecorder().SetGauge(deps.GaugeConsecutiveErrors, 1.0)

			obs := wb.WrapStatusAccumulated(context.Background(), workerTestStatus{Reachable: true})

			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(5)))
			Expect(typed.Metrics.Worker.Gauges["consecutive_errors"]).To(Equal(1.0))
		})

		It("handles uninitialized WorkerBase gracefully (no panic)", func() {
			uninit := &fsmv2.WorkerBase[workerTestConfig, workerTestStatus]{}
			Expect(func() {
				obs := uninit.WrapStatusAccumulated(context.Background(), workerTestStatus{})
				Expect(obs).NotTo(BeNil())
			}).NotTo(Panic())
		})

		It("sets CollectedAt and Status correctly", func() {
			sr := &configurableStateReader{err: fmt.Errorf("no state")}
			wb.InitBase(identity, mockLogger, sr)

			before := time.Now()
			status := workerTestStatus{Reachable: true, LatencyMs: 42}
			obs := wb.WrapStatusAccumulated(context.Background(), status)

			Expect(obs.GetTimestamp()).To(BeTemporally(">=", before))
			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Status.Reachable).To(BeTrue())
			Expect(typed.Status.LatencyMs).To(Equal(int64(42)))
		})

		It("copies framework metrics from baseDeps", func() {
			sr := &configurableStateReader{err: fmt.Errorf("no state")}
			wb.InitBase(identity, mockLogger, sr)

			obs := wb.WrapStatusAccumulated(context.Background(), workerTestStatus{})
			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Metrics.Framework.StateTransitionsTotal).To(Equal(int64(0)))
		})

		It("calls LoadObservedTyped with correct workerType and ID", func() {
			sr := &configurableStateReader{err: fmt.Errorf("no state")}
			wb.InitBase(identity, mockLogger, sr)

			_ = wb.WrapStatusAccumulated(context.Background(), workerTestStatus{})

			Expect(sr.called).To(BeTrue())
			Expect(sr.calledType).To(Equal("test"))
			Expect(sr.calledID).To(Equal("test-worker-1"))
		})

		It("handles nil stateReader gracefully", func() {
			wb.InitBase(identity, mockLogger, nil)

			bd := wb.GetDependenciesAny().(*deps.BaseDependencies)
			bd.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 3)

			obs := wb.WrapStatusAccumulated(context.Background(), workerTestStatus{})

			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(3)))
		})

		It("copies action history from baseDeps", func() {
			sr := &configurableStateReader{err: fmt.Errorf("no state")}
			wb.InitBase(identity, mockLogger, sr)

			bd := wb.GetDependenciesAny().(*deps.BaseDependencies)
			bd.SetActionHistory([]deps.ActionResult{
				{ActionType: "test-action", Success: true},
			})

			obs := wb.WrapStatusAccumulated(context.Background(), workerTestStatus{})
			typed := obs.(fsmv2.Observation[workerTestStatus])
			Expect(typed.LastActionResults).To(HaveLen(1))
			Expect(typed.LastActionResults[0].ActionType).To(Equal("test-action"))
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			fsmv2.RegisterInitialState("test", &testStoppedState{})
		})

		AfterEach(func() {
			fsmv2.ResetInitialStateRegistry()
		})

		It("returns the registered state for the worker type", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)
			state := wb.GetInitialState()
			Expect(state).NotTo(BeNil())
			Expect(state.String()).To(Equal("Stopped"))
		})

		It("panics when no state is registered for the worker type", func() {
			fsmv2.ResetInitialStateRegistry()
			wb.InitBase(identity, mockLogger, mockStateReader)
			Expect(func() {
				wb.GetInitialState()
			}).To(PanicWith(ContainSubstring("no initial state registered")))
		})

		It("panics when InitBase was not called", func() {
			uninit := &fsmv2.WorkerBase[workerTestConfig, workerTestStatus]{}
			Expect(func() {
				uninit.GetInitialState()
			}).To(PanicWith(ContainSubstring("InitBase was not called")))
		})
	})

	Describe("GetDependenciesAny", func() {
		It("returns baseDeps after initialization", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)
			d := wb.GetDependenciesAny()
			Expect(d).NotTo(BeNil())

			baseDeps, ok := d.(*deps.BaseDependencies)
			Expect(ok).To(BeTrue())
			Expect(baseDeps).NotTo(BeNil())
		})

		It("returns nil before initialization", func() {
			uninit := &fsmv2.WorkerBase[workerTestConfig, workerTestStatus]{}
			Expect(uninit.GetDependenciesAny()).To(BeNil())
		})
	})

	Describe("thread safety", func() {
		It("is safe for concurrent DeriveDesiredState and Config access", func() {
			wb.InitBase(identity, mockLogger, mockStateReader)

			var wg sync.WaitGroup
			stop := make(chan struct{})

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer GinkgoRecover()
					spec := config.UserSpec{
						Config: "host: \"localhost\"\nport: 8080",
					}
					for {
						select {
						case <-stop:
							return
						default:
							_, _ = wb.DeriveDesiredState(spec)
						}
					}
				}()
			}

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer GinkgoRecover()
					for {
						select {
						case <-stop:
							return
						default:
							_ = wb.Config()
							_ = wb.ConfigReady()
						}
					}
				}()
			}

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer GinkgoRecover()
					for {
						select {
						case <-stop:
							return
						default:
							_ = wb.WrapStatus(workerTestStatus{})
						}
					}
				}()
			}

			time.Sleep(100 * time.Millisecond)
			close(stop)
			wg.Wait()
		})
	})

	Describe("L5 invariant", func() {
		It("does not satisfy ActionProvider", func() {
			_, ok := any(wb).(interface {
				Actions() map[string]fsmv2.Action[any]
			})
			Expect(ok).To(BeFalse(), "WorkerBase must not satisfy ActionProvider (L5)")
		})

		It("does not satisfy ChildSpecProvider", func() {
			_, ok := any(wb).(interface {
				ChildSpecs() []config.ChildSpec
			})
			Expect(ok).To(BeFalse(), "WorkerBase must not satisfy ChildSpecProvider (L5)")
		})

		It("does not satisfy MetricsProvider", func() {
			_, ok := any(wb).(interface {
				Metrics() []prometheus.Collector
			})
			Expect(ok).To(BeFalse(), "WorkerBase must not satisfy MetricsProvider (L5)")
		})

		It("does not satisfy GracefulShutdowner", func() {
			_, ok := any(wb).(interface {
				Shutdown(ctx context.Context) error
			})
			Expect(ok).To(BeFalse(), "WorkerBase must not satisfy GracefulShutdowner (L5)")
		})

		It("does not satisfy ChildrenViewConsumer", func() {
			_, ok := any(wb).(interface {
				SetChildrenView(view any)
			})
			Expect(ok).To(BeFalse(), "WorkerBase must not satisfy ChildrenViewConsumer (L5)")
		})
	})
})
