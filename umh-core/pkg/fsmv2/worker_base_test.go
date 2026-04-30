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

// configWithUserSpec embeds BaseUserSpec inline alongside custom fields.
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

		It("parses YAML into config that embeds BaseUserSpec", func() {
			sgWb := &fsmv2.WorkerBase[stateGetterConfig, workerTestStatus]{}
			sgWb.InitBase(identity, mockLogger, mockStateReader)

			spec := config.UserSpec{
				Config: `state: "stopped"
host: "example.com"`,
			}
			ds, err := sgWb.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			typed := ds.(*fsmv2.WrappedDesiredState[stateGetterConfig])
			Expect(typed.Config.Host).To(Equal("example.com"))
			Expect(typed.Config.GetState()).To(Equal("stopped"))
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
				SetChildrenView(view config.ChildrenView) fsmv2.ObservedState
			})
			Expect(ok).To(BeFalse(), "WorkerBase must not satisfy ChildrenViewConsumer (L5)")
		})
	})
})
