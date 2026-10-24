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
	"encoding/json"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/fsmv2_adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/hook"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
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
func localHook(fs ...func(hook.Deps) (hook.Result, error)) func(hook.Deps) (hook.Result, error) {
	return func(d hook.Deps) (hook.Result, error) {
		var result hook.Result
		for _, f := range fs {
			r, err := f(d)
			if err != nil {
				return hook.Result{}, err
			}
			result.Children = append(result.Children, r.Children...)
			if r.Status != nil {
				result.Status = r.Status
			}
		}
		return result, nil
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
	wireFSMv2CommunicatorFn = func(context.Context, fsmv2Supervisor, *fsmv2_adapter.LegacyChannelBridge, storage.TriangularStoreInterface, string, *config.FullConfig, *communication_state.CommunicationState, *zap.SugaredLogger, hook.StatusFunc) {
	}
}

// installStubBuildWithBridge makes the build seam return a stub supervisor
// and a real, started channel bridge, so the real communicator wiring can
// deliver status messages the spec can read.
func installStubBuildWithBridge(ctx context.Context) *fsmv2_adapter.LegacyChannelBridge {
	newFSMv2Store = func(l deps.FSMLogger) storage.TriangularStoreInterface {
		return scenariorunner.SetupStore(l)
	}
	bridge := fsmv2_adapter.NewLegacyChannelBridge(
		make(chan *models.UMHMessage, 100),
		make(chan *models.UMHMessage, 100),
		zap.NewNop().Sugar(),
		0,
		nil,
	)
	bridge.Start(ctx)
	buildFSMv2SupervisorFn = func(ctx context.Context, configData *config.FullConfig, communicationState *communication_state.CommunicationState, logger *zap.SugaredLogger, forceExit <-chan struct{}) (fsmv2Supervisor, *fsmv2_adapter.LegacyChannelBridge, storage.TriangularStoreInterface, string, func(), error) {
		store := newFSMv2Store(deps.NewFSMLogger(logger))
		return &stubSupervisor{}, bridge, store, "placeholder-uuid", func() {}, nil
	}

	return bridge
}

// runStartup drives the real startup function with the standard stubs and
// returns its error.
func runStartup(cfg *config.FullConfig) (fsmv2StartupResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return startFSMv2Transport(ctx, cfg, nil, zap.NewNop().Sugar(), nil)
}

// runStartupWithState drives the real startup function with the standard
// stubs and a real communication state, so the real communicator wiring runs.
func runStartupWithState(ctx context.Context, cfg *config.FullConfig, commState *communication_state.CommunicationState) (fsmv2StartupResult, error) {
	return startFSMv2Transport(ctx, cfg, commState, zap.NewNop().Sugar(), nil)
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
		runHook = localHook(func(hook.Deps) (hook.Result, error) {
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
			return hook.Result{}, nil
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

		runHook = localHook(func(hook.Deps) (hook.Result, error) {
			return hook.Result{}, errHook
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

		runHook = localHook(func(hook.Deps) (hook.Result, error) { return hook.Result{}, nil })

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
			runHook = localHook(func(hook.Deps) (hook.Result, error) { return hook.Result{}, nil })
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

			runHook = localHook(func(hook.Deps) (hook.Result, error) { return hook.Result{}, nil })

			transport, err := runStartup(cfg)
			Expect(err).NotTo(HaveOccurred())
			defer transport.cleanup()
			Eventually(supStarted, time.Second).Should(Receive())
		})
	})

	// newStartupCommunicationState builds the communication state main would
	// build: a watchdog, the legacy channels, a snapshot manager with one
	// manager, a mock config manager, and no topic browser simulator.
	newStartupCommunicationState := func(ctx context.Context) *communication_state.CommunicationState {
		snapshotManager := fsm.NewSnapshotManager()
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			SnapshotTime: time.Now(),
			Managers: map[string]fsm.ManagerSnapshot{
				constants.ContainerManagerName: &fsm.BaseManagerSnapshot{
					Name:      constants.ContainerManagerName,
					Instances: map[string]*fsm.FSMInstanceSnapshot{},
				},
			},
		})

		return communication_state.NewCommunicationState(
			watchdog.NewWatchdog(ctx, time.NewTicker(10*time.Second), true, zap.NewNop().Sugar()),
			make(chan *models.UMHMessage, 100),
			make(chan *models.UMHMessage, 100),
			config.ReleaseChannelStable,
			snapshotManager,
			config.NewMockConfigManager(),
			zap.NewNop().Sugar(),
			topicbrowser.NewCache(),
			nil,
		)
	}

	// decodedConnectionPart reads the next status message off the bridge's
	// outbound channel and returns its connection part, nil when the message
	// carries none.
	decodedConnectionPart := func(bridge *fsmv2_adapter.LegacyChannelBridge) *models.Communicator {
		_, outbound := bridge.GetChannels("")
		var transportMsg *types.UMHMessage
		Eventually(outbound, "10s", "100ms").Should(Receive(&transportMsg))

		content, err := encoding.DecodeMessageFromUMHInstanceToUser(transportMsg.Content)
		Expect(err).NotTo(HaveOccurred())

		rawPayload, err := json.Marshal(content.Payload)
		Expect(err).NotTo(HaveOccurred())

		var status models.StatusMessage
		Expect(json.Unmarshal(rawPayload, &status)).To(Succeed())

		return status.Core.Communicator
	}

	It("checks that a registered Status reaches the status message through main's startup", func() {
		// No fsmv2 client is published in this spec, so nothing but the
		// registered Status can fill the connection part.
		previousClient := fsmv2client.GetClient()
		fsmv2client.SetClient(nil)
		DeferCleanup(func() { fsmv2client.SetClient(previousClient) })

		p := &models.Communicator{State: "Running", SubscriberCount: 5}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		commState := newStartupCommunicationState(ctx)
		bridge := installStubBuildWithBridge(ctx)
		installStartProbe(make(chan struct{}, 1))

		runHook = func(hook.Deps) (hook.Result, error) {
			return hook.Result{Status: func(context.Context, int) *models.Communicator { return p }}, nil
		}

		_, err := runStartupWithState(ctx, cfg, commState)
		Expect(err).NotTo(HaveOccurred())

		// The wire goroutine creates the handler after startup returns. Poll
		// the lock-taking accessor until it exists; the handler stays for the
		// rest of the spec, so this waits for a state, not a window.
		Eventually(commState.GetSubscriberHandler, "10s", "10ms").ShouldNot(BeNil())
		commState.GetSubscriberHandler().AddOrRefreshSubscriber("status@example.com", false)

		// The provider's value survives the JSON round trip, so compare
		// against the round-tripped form of p.
		expected := &models.Communicator{}
		raw, err := json.Marshal(p)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(raw, expected)).To(Succeed())

		Expect(decodedConnectionPart(bridge)).To(Equal(expected),
			"the registered Status must reach the status message's connection part")
	})

	It("checks that no registered Status leaves the status message's connection part empty", func() {
		previousClient := fsmv2client.GetClient()
		fsmv2client.SetClient(nil)
		DeferCleanup(func() { fsmv2client.SetClient(previousClient) })

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		commState := newStartupCommunicationState(ctx)
		bridge := installStubBuildWithBridge(ctx)
		installStartProbe(make(chan struct{}, 1))

		runHook = localHook()

		_, err := runStartupWithState(ctx, cfg, commState)
		Expect(err).NotTo(HaveOccurred())

		// Same readiness wait as the spec above: the wire goroutine creates
		// the handler after startup returns.
		Eventually(commState.GetSubscriberHandler, "10s", "10ms").ShouldNot(BeNil())
		commState.GetSubscriberHandler().AddOrRefreshSubscriber("empty@example.com", false)

		Expect(decodedConnectionPart(bridge)).To(BeNil(),
			"with no registered Status the connection part must stay empty")
	})
})
