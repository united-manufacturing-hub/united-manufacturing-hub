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

package main

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/fsmv2_adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/hook"
)

// stubSupervisor is a fake fsmv2Supervisor whose Run blocks until the context
// is cancelled.
type stubSupervisor struct{}

func (s *stubSupervisor) GetDebugInfo() interface{} { return nil }

func (s *stubSupervisor) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// localHook builds a spec-local registration run with the same semantics as
// hook.Run: it calls each f in order with the same Deps, and returns the
// first non-nil error without calling the rest. The cmd specs never touch
// hook's package-global registry, so no spec's functions can leak into
// another's run.
func localHook(fs ...func(hook.Deps) error) func(hook.Deps) error {
	return func(d hook.Deps) error {
		for _, f := range fs {
			if err := f(d); err != nil {
				return err
			}
		}
		return nil
	}
}

// restoreStartupSeams puts every startup seam back to its production
// implementation, so one spec's probes cannot leak into the next.
func restoreStartupSeams() {
	runHook = hook.Run
	newFSMv2Store = scenariorunner.SetupStore
	buildFSMv2SupervisorFn = buildFSMv2Supervisor
	startFSMv2SupervisorFn = defaultStartFSMv2Supervisor
	wireFSMv2CommunicatorFn = wireFSMv2Communicator
}

// installStubBuild makes the build seam return a stub supervisor and a real
// store built through the store seam, so a spec can drive the real startup
// path without the real supervisor tree.
func installStubBuild() {
	newFSMv2Store = func(l deps.FSMLogger) storage.TriangularStoreInterface {
		return scenariorunner.SetupStore(l)
	}
	buildFSMv2SupervisorFn = func(ctx context.Context, configData *config.FullConfig, communicationState *communication_state.CommunicationState, logger *zap.SugaredLogger, forceExit <-chan struct{}) (fsmv2Supervisor, *fsmv2_adapter.LegacyChannelBridge, storage.TriangularStoreInterface, string, func(), error) {
		store := newFSMv2Store(deps.NewFSMLogger(logger))
		return &stubSupervisor{}, nil, store, "placeholder-uuid", func() {}, nil
	}
}

// installStartProbe makes the start seam record that the supervisor started
// and close done.
func installStartProbe(supStarted chan struct{}) {
	startFSMv2SupervisorFn = func(appSup fsmv2Supervisor, ctx context.Context, done chan struct{}, log *zap.SugaredLogger) {
		supStarted <- struct{}{}
		close(done)
	}
}

// noopWire replaces the communicator wiring with a no-op.
func noopWire() {
	wireFSMv2CommunicatorFn = func(context.Context, fsmv2Supervisor, *fsmv2_adapter.LegacyChannelBridge, storage.TriangularStoreInterface, string, *config.FullConfig, *communication_state.CommunicationState, *zap.SugaredLogger) {
	}
}

// runStartup drives the real startup function with the standard stubs and
// returns its error.
func runStartup(cfg *config.FullConfig) (fsmv2StartupResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return startFSMv2Transport(ctx, cfg, nil, zap.NewNop().Sugar(), nil)
}

var _ = Describe("Startup seams", func() {
	var cfg *config.FullConfig

	BeforeEach(func() {
		restoreStartupSeams()
		cfg = &config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.invalid",
					AuthToken: "test-token",
				},
			},
		}
	})

	AfterEach(func() {
		restoreStartupSeams()
	})

	It("checks that startup runs the hook before the supervisor starts", func() {
		supStarted := make(chan struct{}, 1)

		installStubBuild()
		noopWire()
		installStartProbe(supStarted)

		record := make(chan string, 1)
		runHook = localHook(func(hook.Deps) error {
			msg := "not started"
			select {
			case <-supStarted:
				msg = "started"
			default:
			}

			select {
			case record <- msg:
			default:
			}
			return nil
		})

		transport, err := runStartup(cfg)
		Expect(err).NotTo(HaveOccurred())
		defer transport.cleanup()

		Expect(record).To(Receive(Equal("not started")))
	})

	It("checks that startup returns the hook's error and never starts the supervisor", func() {
		errHook := errors.New("planted hook error")

		supStarted := make(chan struct{}, 1)
		installStubBuild()
		noopWire()
		installStartProbe(supStarted)

		runHook = localHook(func(hook.Deps) error {
			return errHook
		})

		_, err := runStartup(cfg)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, errHook)).To(BeTrue())
		Expect(supStarted).NotTo(Receive())
	})

	It("checks that startup starts the supervisor when the hook returns nil", func() {
		supStarted := make(chan struct{}, 1)
		installStubBuild()
		noopWire()
		installStartProbe(supStarted)

		runHook = localHook(func(hook.Deps) error { return nil })

		transport, err := runStartup(cfg)
		Expect(err).NotTo(HaveOccurred())
		defer transport.cleanup()
		Eventually(supStarted, time.Second).Should(Receive())
	})

	Context("when the FSMv2 build fails", func() {
		buildErr := errors.New("planted build error")

		BeforeEach(func() {
			buildFSMv2SupervisorFn = func(ctx context.Context, configData *config.FullConfig, communicationState *communication_state.CommunicationState, logger *zap.SugaredLogger, forceExit <-chan struct{}) (fsmv2Supervisor, *fsmv2_adapter.LegacyChannelBridge, storage.TriangularStoreInterface, string, func(), error) {
				return nil, nil, nil, "", func() {}, buildErr
			}
			noopWire()
		})

		It("checks that startup returns the build error when nothing is registered (the OSS build)", func() {
			supStarted := make(chan struct{}, 1)
			installStartProbe(supStarted)

			_, err := runStartup(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, buildErr)).To(BeTrue())
			Consistently(supStarted, 200*time.Millisecond).ShouldNot(Receive())
		})

		It("checks that startup returns the build error when one function is registered (the official image)", func() {
			runHook = localHook(func(hook.Deps) error { return nil })
			supStarted := make(chan struct{}, 1)
			installStartProbe(supStarted)

			_, err := runStartup(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, buildErr)).To(BeTrue())
			Consistently(supStarted, 200*time.Millisecond).ShouldNot(Receive())
		})

		It("checks that a good build starts the supervisor when nothing is registered (the OSS build)", func() {
			supStarted := make(chan struct{}, 1)
			installStubBuild()
			installStartProbe(supStarted)

			transport, err := runStartup(cfg)
			Expect(err).NotTo(HaveOccurred())
			defer transport.cleanup()
			Eventually(supStarted, time.Second).Should(Receive())
		})

		It("checks that a good build starts the supervisor when one function is registered (the official image)", func() {
			supStarted := make(chan struct{}, 1)
			installStubBuild()
			installStartProbe(supStarted)

			runHook = localHook(func(hook.Deps) error { return nil })

			transport, err := runStartup(cfg)
			Expect(err).NotTo(HaveOccurred())
			defer transport.cleanup()
			Eventually(supStarted, time.Second).Should(Receive())
		})
	})
})
