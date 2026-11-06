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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/scenariorunner"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
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

// nothingRegistered stands in for the open-source build, where no Management
// Console module is registered.
func nothingRegistered(managementconsole.Deps) (managementconsole.Result, error) {
	return managementconsole.Result{}, nil
}

// registeredModule stands in for the official image, where the Management
// Console module is registered and returns its children and a Status.
func registeredModule(children []fsmv2config.ChildSpec, status managementconsole.StatusFunc) managementconsole.StartFunc {
	return func(managementconsole.Deps) (managementconsole.Result, error) {
		return managementconsole.Result{Children: children, Status: status}, nil
	}
}

// restoreStartupSeams puts every startup seam back to its production
// implementation, so one spec's probes cannot leak into the next.
func restoreStartupSeams() {
	startManagementConsoleConnection = linkedManagementConsoleModule
	newFSMv2Store = scenariorunner.SetupStore
	buildFSMv2SupervisorFn = buildFSMv2Supervisor
	runFSMv2SupervisorFn = runFSMv2SupervisorInBackground
	runRouterFn = runRouter
}

// installStubBuild makes the build seam return a stub supervisor, after
// building a real store through the store seam, so a spec can drive the real
// startup path without the real supervisor tree. It returns where the build
// records the children it was given.
func installStubBuild() *[]fsmv2config.ChildSpec {
	var got []fsmv2config.ChildSpec

	newFSMv2Store = func(l deps.FSMLogger) storage.TriangularStoreInterface {
		return scenariorunner.SetupStore(l)
	}
	buildFSMv2SupervisorFn = func(configManager config.ConfigManager, cpuMonitorEnabled bool, memoryMonitorEnabled bool, logger *zap.SugaredLogger, forceExit <-chan struct{}, registered []fsmv2config.ChildSpec) (fsmv2Supervisor, func(), error) {
		_ = newFSMv2Store(deps.NewFSMLogger(logger))
		got = registered

		return &stubSupervisor{}, func() {}, nil
	}

	return &got
}

// installStartProbe makes the run seam record that the supervisor started
// and close stopped.
func installStartProbe(supStarted chan struct{}) {
	runFSMv2SupervisorFn = func(ctx context.Context, applicationSupervisor fsmv2Supervisor, stopped chan struct{}, log *zap.SugaredLogger) {
		supStarted <- struct{}{}
		close(stopped)
	}
}

// noopRouter replaces runRouter with a no-op.
func noopRouter() {
	runRouterFn = func(context.Context, <-chan handoff.Message, chan<- handoff.Message, *config.FullConfig, *communication_state.CommunicationState, *zap.SugaredLogger, managementconsole.StatusFunc) {
	}
}

// newBareCommunicationState is the smallest communication state startup
// accepts: it records the login response and nothing else.
func newBareCommunicationState() *communication_state.CommunicationState {
	return communication_state.NewCommunicationState(nil, nil, nil, config.ReleaseChannelStable, nil, nil, zap.NewNop().Sugar(), nil, nil)
}

// runStartup drives the real startup function with the standard stubs and
// returns its error.
func runStartup(cfg *config.FullConfig) (fsmv2Handle, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return startFSMv2Runtime(ctx, cfg, newBareCommunicationState(), zap.NewNop().Sugar(), nil)
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
		noopRouter()
		installStartProbe(supStarted)

		record := make(chan string, 1)
		startManagementConsoleConnection = func(managementconsole.Deps) (managementconsole.Result, error) {
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

			return managementconsole.Result{}, nil
		}

		handle, err := runStartup(cfg)
		Expect(err).NotTo(HaveOccurred())
		defer handle.cleanup()

		Expect(record).To(Receive(Equal("not started")))
	})

	It("checks that startup returns the hook's error and never starts the supervisor", func() {
		errHook := errors.New("planted hook error")

		supStarted := make(chan struct{}, 1)
		installStubBuild()
		noopRouter()
		installStartProbe(supStarted)

		startManagementConsoleConnection = func(managementconsole.Deps) (managementconsole.Result, error) {
			return managementconsole.Result{}, errHook
		}

		_, err := runStartup(cfg)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, errHook)).To(BeTrue())
		Expect(supStarted).NotTo(Receive())
	})

	It("checks that startup starts the supervisor when the hook returns nil", func() {
		supStarted := make(chan struct{}, 1)
		installStubBuild()
		noopRouter()
		installStartProbe(supStarted)

		startManagementConsoleConnection = nothingRegistered

		handle, err := runStartup(cfg)
		Expect(err).NotTo(HaveOccurred())
		defer handle.cleanup()
		Eventually(supStarted, time.Second).Should(Receive())
	})

	It("checks that startup logs that no module is linked and starts the supervisor without children when credentials are set", func() {
		got := installStubBuild()
		noopRouter()
		supStarted := make(chan struct{}, 1)
		installStartProbe(supStarted)

		startManagementConsoleConnection = nil

		core, logs := observer.New(zap.InfoLevel)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		handle, err := startFSMv2Runtime(ctx, cfg, newBareCommunicationState(), zap.New(core).Sugar(), nil)
		Expect(err).NotTo(HaveOccurred())
		defer handle.cleanup()

		Expect(logs.FilterMessage("no Management Console module linked").Len()).To(Equal(1))
		Expect(*got).To(BeEmpty())
		Eventually(supStarted, time.Second).Should(Receive())
	})

	It("checks that startup hands the module's children to the supervisor build", func() {
		got := installStubBuild()
		noopRouter()
		installStartProbe(make(chan struct{}, 1))

		child := fsmv2config.ChildSpec{Name: "x", WorkerType: "test-worker", Enabled: true}
		startManagementConsoleConnection = func(managementconsole.Deps) (managementconsole.Result, error) {
			return managementconsole.Result{Children: []fsmv2config.ChildSpec{child}}, nil
		}

		handle, err := runStartup(cfg)
		Expect(err).NotTo(HaveOccurred())
		defer handle.cleanup()

		Expect(*got).To(Equal([]fsmv2config.ChildSpec{child}))
	})

	It("checks that startup hands the module the relay settings", func() {
		installStubBuild()
		noopRouter()
		installStartProbe(make(chan struct{}, 1))

		cfg.Agent.AllowInsecureTLS = true
		cfg.Agent.Location = map[int]string{0: "enterprise"}

		var got managementconsole.Config
		startManagementConsoleConnection = func(d managementconsole.Deps) (managementconsole.Result, error) {
			got = d.Config

			return managementconsole.Result{}, nil
		}

		handle, err := runStartup(cfg)
		Expect(err).NotTo(HaveOccurred())
		defer handle.cleanup()

		_, err = uuid.Parse(got.LoginPlaceholderUUID)
		Expect(err).NotTo(HaveOccurred(), "the login UUID must be a UUID")

		got.LoginPlaceholderUUID = ""
		Expect(got).To(Equal(managementconsole.Config{
			RelayURL:         "https://example.invalid",
			AuthToken:        "test-token",
			AllowInsecureTLS: true,
			Location:         map[int]string{0: "enterprise"},
		}))
	})

	It("checks that the instance UUID the module reports becomes the login response's UUID", func() {
		installStubBuild()
		noopRouter()
		installStartProbe(make(chan struct{}, 1))

		var report func(string)
		startManagementConsoleConnection = func(d managementconsole.Deps) (managementconsole.Result, error) {
			report = d.SetInstanceUUID

			return managementconsole.Result{}, nil
		}

		commState := newBareCommunicationState()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		handle, err := startFSMv2Runtime(ctx, cfg, commState, zap.NewNop().Sugar(), nil)
		Expect(err).NotTo(HaveOccurred())
		defer handle.cleanup()

		reported := uuid.New()
		report(reported.String())

		commState.LoginResponseMu.RLock()
		got := commState.LoginResponse.UUID
		commState.LoginResponseMu.RUnlock()
		Expect(got).To(Equal(reported))
	})

	Context("when the FSMv2 build fails", func() {
		buildErr := errors.New("planted build error")
		stubChildren := []fsmv2config.ChildSpec{{Name: "connection", WorkerType: "test-worker", Enabled: true}}
		stubStatus := func(context.Context) *managementconsole.Status { return nil }

		BeforeEach(func() {
			buildFSMv2SupervisorFn = func(configManager config.ConfigManager, cpuMonitorEnabled bool, memoryMonitorEnabled bool, logger *zap.SugaredLogger, forceExit <-chan struct{}, registered []fsmv2config.ChildSpec) (fsmv2Supervisor, func(), error) {
				return nil, func() {}, buildErr
			}
			noopRouter()
		})

		It("checks that startup returns the build error when nothing is registered (the OSS build)", func() {
			supStarted := make(chan struct{}, 1)
			installStartProbe(supStarted)

			_, err := runStartup(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, buildErr)).To(BeTrue())
			Consistently(supStarted, 200*time.Millisecond).ShouldNot(Receive())
		})

		It("checks that startup returns the build error when a module is registered (the official image)", func() {
			startManagementConsoleConnection = registeredModule(stubChildren, stubStatus)
			supStarted := make(chan struct{}, 1)
			installStartProbe(supStarted)

			_, err := runStartup(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, buildErr)).To(BeTrue())
			Consistently(supStarted, 200*time.Millisecond).ShouldNot(Receive())
		})
	})

	Context("when the FSMv2 build succeeds", func() {
		stubChildren := []fsmv2config.ChildSpec{{Name: "connection", WorkerType: "test-worker", Enabled: true}}
		stubStatus := func(context.Context) *managementconsole.Status { return nil }

		BeforeEach(func() {
			noopRouter()
		})

		It("checks that a good build starts the supervisor when nothing is registered (the OSS build)", func() {
			supStarted := make(chan struct{}, 1)
			installStubBuild()
			installStartProbe(supStarted)

			handle, err := runStartup(cfg)
			Expect(err).NotTo(HaveOccurred())
			defer handle.cleanup()
			Eventually(supStarted, time.Second).Should(Receive())
		})

		It("checks that a good build gets the module's children and starts the supervisor when a module is registered (the official image)", func() {
			supStarted := make(chan struct{}, 1)
			got := installStubBuild()
			installStartProbe(supStarted)

			startManagementConsoleConnection = registeredModule(stubChildren, stubStatus)

			handle, err := runStartup(cfg)
			Expect(err).NotTo(HaveOccurred())
			defer handle.cleanup()
			Expect(*got).To(Equal(stubChildren))
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

	// decodedConnectionPart reads the next status message off the module's
	// outbound channel and returns its connection part, nil when the message
	// carries none.
	decodedConnectionPart := func(outbound <-chan handoff.Message) *models.Communicator {
		var msg handoff.Message
		Eventually(outbound, "10s", "100ms").Should(Receive(&msg))

		content, err := encoding.DecodeMessageFromUMHInstanceToUser(string(msg.Content))
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

		p := &managementconsole.Status{Health: managementconsole.Active, HealthMessage: "connected", State: "Running"}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		commState := newStartupCommunicationState(ctx)
		installStubBuild()
		installStartProbe(make(chan struct{}, 1))

		var outbound <-chan handoff.Message
		startManagementConsoleConnection = func(d managementconsole.Deps) (managementconsole.Result, error) {
			outbound = d.Outbound

			return managementconsole.Result{Status: func(context.Context) *managementconsole.Status { return p }}, nil
		}

		_, err := startFSMv2Runtime(ctx, cfg, commState, zap.NewNop().Sugar(), nil)
		Expect(err).NotTo(HaveOccurred())

		// runRouter creates the handler before startup returns.
		Expect(commState.GetSubscriberHandler()).NotTo(BeNil())
		commState.GetSubscriberHandler().AddOrRefreshSubscriber("status@example.com", false)

		Expect(decodedConnectionPart(outbound)).To(Equal(&models.Communicator{
			Health:          &models.Health{Category: models.Active, Message: "connected", ObservedState: models.Active.String(), DesiredState: models.Active.String()},
			State:           "Running",
			SubscriberCount: 1,
		}), "the registered Status and umh-core's subscriber count must reach the status message's connection part")
	})

	It("checks that no registered Status leaves the status message's connection part empty", func() {
		previousClient := fsmv2client.GetClient()
		fsmv2client.SetClient(nil)
		DeferCleanup(func() { fsmv2client.SetClient(previousClient) })

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		commState := newStartupCommunicationState(ctx)
		installStubBuild()
		installStartProbe(make(chan struct{}, 1))

		var outbound <-chan handoff.Message
		startManagementConsoleConnection = func(d managementconsole.Deps) (managementconsole.Result, error) {
			outbound = d.Outbound

			return managementconsole.Result{}, nil
		}

		_, err := startFSMv2Runtime(ctx, cfg, commState, zap.NewNop().Sugar(), nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(commState.GetSubscriberHandler()).NotTo(BeNil())
		commState.GetSubscriberHandler().AddOrRefreshSubscriber("empty@example.com", false)

		Expect(decodedConnectionPart(outbound)).To(BeNil(),
			"with no registered Status the connection part must stay empty")
	})
})

var _ = Describe("forwardReplies", func() {
	It("checks that an action reply reaches the module as its content, recipient and trace ID", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		replies := make(chan *models.UMHMessage, 1)
		outbound := make(chan handoff.Message, 1)
		go forwardReplies(ctx, replies, outbound)

		trace := uuid.New()
		replies <- &models.UMHMessage{Content: "encoded-reply", Email: "user@example.com", Metadata: &models.MessageMetadata{TraceID: trace}}

		var got handoff.Message
		Eventually(outbound, time.Second).Should(Receive(&got))
		Expect(got).To(Equal(handoff.Message{Content: []byte("encoded-reply"), SenderEmail: "user@example.com", TraceID: trace.String()}))
	})

	It("checks that a reply without a trace ID reaches the module with an empty trace ID", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		replies := make(chan *models.UMHMessage, 1)
		outbound := make(chan handoff.Message, 1)
		go forwardReplies(ctx, replies, outbound)

		replies <- &models.UMHMessage{Content: "encoded-reply", Email: "user@example.com"}

		var got handoff.Message
		Eventually(outbound, time.Second).Should(Receive(&got))
		Expect(got.TraceID).To(BeEmpty())
	})

	It("checks that forwardReplies returns when ctx is cancelled while outbound is full", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		replies := make(chan *models.UMHMessage, 1)
		outbound := make(chan handoff.Message, 1)
		outbound <- handoff.Message{Content: []byte("queued")}

		returned := make(chan struct{})
		go func() {
			defer close(returned)
			forwardReplies(ctx, replies, outbound)
		}()

		replies <- &models.UMHMessage{Content: "waiting-reply"}
		Eventually(func() int { return len(replies) }, time.Second).Should(BeZero())
		Consistently(returned, 100*time.Millisecond).ShouldNot(BeClosed())

		cancel()
		Eventually(returned, time.Second).Should(BeClosed())
	})
})
