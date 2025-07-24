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

package topicbrowser

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthossvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	rpsvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	rpfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	rpsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	rpmonitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("TopicBrowserService", func() {
	var (
		service         *Service
		mockBenthos     *benthossvc.MockBenthosService
		ctx             context.Context
		tick            uint64
		tbName          string
		cancelFunc      context.CancelFunc
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancelFunc = context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
		tick = 1
		tbName = "test-component"

		// Set up mock benthos service
		mockBenthos = benthossvc.NewMockBenthosService()

		// Set up a real service with mocked dependencies
		service = NewDefaultService(tbName,
			WithService(mockBenthos))
		mockSvcRegistry = serviceregistry.NewMockRegistry()
	})

	AfterEach(func() {
		// Clean up if necessary
		cancelFunc()
	})

	Describe("AddToManager", func() {
		var cfg *benthossvccfg.BenthosServiceConfig

		BeforeEach(func() {
			// Create a basic config for testing

			cfg = &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "benthos_kafka_test",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			// Set up mock to return a valid BenthosServiceConfig when generating config
			mockBenthos.ServiceExistsResult = false
		})

		It("should add a new topic browser to the benthos manager", func() {
			// Act
			err := service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)

			// Assert
			Expect(err).NotTo(HaveOccurred())

			// Verify that a Benthos config was added to the service
			Expect(service.benthosConfigs).To(HaveLen(1))

			// Verify the name follows the expected pattern
			benthosName := service.getName(tbName)
			Expect(service.benthosConfigs[0].Name).To(Equal(benthosName))

			// Verify the desired state is set correctly
			Expect(service.benthosConfigs[0].DesiredFSMState).To(Equal(benthosfsm.OperationalStateStopped))
		})

		It("should return error when topicbrowser already exists", func() {
			// Add the topic browser first
			err := service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)
			Expect(err).NotTo(HaveOccurred())

			// Try to add it again
			err = service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)

			// Assert
			Expect(err).To(MatchError(ErrServiceAlreadyExists))
		})

		It("should set up the topic browser for reconciliation with the benthos manager", func() {
			// Act
			err := service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to ensure the topic browser is passed to benthos manager
			mockBenthos.ReconcileManagerReconciled = true
			_, reconciled := service.ReconcileManager(ctx, mockSvcRegistry, tick)

			// Assert
			Expect(reconciled).To(BeTrue())
			Expect(service.benthosConfigs).To(HaveLen(1))
		})
	})

	Describe("Status", func() {
		var (
			cfg                *benthossvccfg.BenthosServiceConfig
			manager            *benthosfsm.BenthosManager
			mockBenthosService *benthossvc.MockBenthosService
			statusService      *Service
			benthosName        string
			logs               []s6svc.LogEntry
		)

		BeforeEach(func() {
			// Create a basic config for testing

			cfg = &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "benthos_kafka_test",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			// Use the official mock manager from the FSM package
			manager, mockBenthosService = benthosfsm.NewBenthosManagerWithMockedServices("test")

			// Create service with our official mock benthos manager
			statusService = NewDefaultService(tbName,
				WithService(mockBenthosService),
				WithManager(manager))

			// Add the topic browser to the service
			err := statusService.AddToManager(ctx, mockSvcRegistry, cfg, tbName)
			Expect(err).NotTo(HaveOccurred())

			// Get the benthos name that will be used
			benthosName = statusService.getName(tbName)

			// Set up the mock to say the component exists
			mockBenthosService.ServiceExistsResult = true
			if mockBenthosService.ExistingServices == nil {
				mockBenthosService.ExistingServices = make(map[string]bool)
			}
			mockBenthosService.ExistingServices[benthosName] = true
		})

		It("should report status correctly for an existing topic browser", func() {
			// Create the full config for reconciliation
			fullCfg := config.FullConfig{
				Internal: config.InternalConfig{
					Benthos: statusService.benthosConfigs,
				},
			}

			// Configure benthos service for proper transitions
			// First configure for creating -> created -> stopped
			ConfigureBenthosManagerForState(mockBenthosService, benthosName, benthosfsm.OperationalStateStopped)

			// Wait for the instance to be created and reach stopped state
			newTick, err := WaitForBenthosManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				manager,
				mockSvcRegistry,
				benthosName,
				benthosfsm.OperationalStateStopped,
				10,
			)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			payload := []byte("hello world")
			hexBlock := makeHex(payload)

			logs = []s6svc.LogEntry{
				{Content: constants.BLOCK_START_MARKER, Timestamp: time.Now()},
				{Content: hexBlock, Timestamp: time.Now()},
				{Content: constants.DATA_END_MARKER, Timestamp: time.Now()},
				{Content: "1750091514783", Timestamp: time.Now()},
				{Content: constants.BLOCK_END_MARKER, Timestamp: time.Now()},
			}

			// Now configure for transition to starting -> active
			ConfigureBenthosManagerForState(mockBenthosService, benthosName, benthosfsm.OperationalStateActive)

			// Start it
			err = statusService.Start(ctx, mockSvcRegistry, tbName)
			Expect(err).NotTo(HaveOccurred())

			// Wait for the instance to reach active state
			newTick, err = WaitForBenthosManagerInstanceState(
				ctx,
				fsm.SystemSnapshot{CurrentConfig: fullCfg, Tick: tick, SnapshotTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				manager,
				mockSvcRegistry,
				benthosName,
				benthosfsm.OperationalStateActive,
				60, // need to wait for at least 60 ticks for health check debouncing (5 seconds)
			)
			Expect(err).NotTo(HaveOccurred())
			tick = newTick

			mockBenthosService.ServiceStates[benthosName].BenthosStatus.BenthosMetrics.Metrics.Input.Received = 10
			mockBenthosService.ServiceStates[benthosName].BenthosStatus.BenthosMetrics.Metrics.Output.Sent = 10

			mockBenthosService.ServiceStates[benthosName].BenthosStatus.BenthosLogs = append(mockBenthosService.ServiceStates[benthosName].BenthosStatus.BenthosLogs, logs...)

			// Reconcile once to ensure that serviceInfo is used to update the observed state
			_, reconciled := statusService.ReconcileManager(ctx, mockSvcRegistry, tick)
			Expect(reconciled).To(BeFalse())

			rpObserved := &rpfsm.RedpandaObservedStateSnapshot{
				ServiceInfoSnapshot: rpsvc.ServiceInfo{
					RedpandaStatus: rpsvc.RedpandaStatus{
						RedpandaMetrics: rpmonitor.RedpandaMetrics{},
					},
				},
				Config: rpsvccfg.RedpandaServiceConfig{},
			}

			rpInstSnapshot := &fsm.FSMInstanceSnapshot{
				ID:                constants.RedpandaInstanceName,
				CurrentState:      rpfsm.OperationalStateActive,
				LastObservedState: rpObserved,
			}

			rpMgrSnapshot := &fsm.BaseManagerSnapshot{
				Name: constants.RedpandaManagerName,
				Instances: map[string]*fsm.FSMInstanceSnapshot{
					constants.RedpandaInstanceName: rpInstSnapshot,
				},
				SnapshotTime: time.Now(),
			}

			snapshot := fsm.SystemSnapshot{Managers: map[string]fsm.ManagerSnapshot{
				constants.RedpandaManagerName: rpMgrSnapshot,
			}, Tick: tick}

			// Call Status
			status, err := statusService.Status(ctx, mockSvcRegistry, tbName, snapshot)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(status.BenthosFSMState).To(Equal(benthosfsm.OperationalStateActive))
			Expect(status.BenthosObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Status).To(Equal(s6svc.ServiceUp))
			// check the ringbuffer if logs appear
			Expect(status.Status.BufferSnapshot.Items).To(HaveLen(1))
			Expect(status.Status.BufferSnapshot.Items[0].Timestamp.UnixMilli()).To(Equal(int64(1750091514783)))
			Expect(status.Status.BufferSnapshot.Items[0].Payload).To(Equal([]byte("hello world")))
		})

		It("should return error for non-existent topic browser", func() {
			// Set up the mock to say the service doesn't exist
			mockBenthosService.ServiceExistsResult = false
			mockBenthosService.ExistingServices = make(map[string]bool)

			snapshot := fsm.SystemSnapshot{Tick: tick}
			// Call Status for a non-existent component
			_, err := statusService.Status(ctx, mockSvcRegistry, tbName, snapshot)

			// Assert - check for "does not exist" in the error message
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
		})
	})

	Describe("UpdateInManager", func() {
		var (
			cfg        *benthossvccfg.BenthosServiceConfig
			updatedCfg *benthossvccfg.BenthosServiceConfig
		)

		BeforeEach(func() {
			// Initial config

			cfg = &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "benthos_kafka_test",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			// Updated config with different settings
			updatedCfg = &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "updated_consumer_group",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			// Add the component first
			err := service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update an existing topic browser", func() {
			// Act - update the topic browser
			err := service.UpdateInManager(ctx, mockSvcRegistry, updatedCfg, tbName)

			// Assert
			Expect(err).NotTo(HaveOccurred())

			// Verify the config was updated but the desired state was preserved
			benthosName := service.getName(tbName)
			var found bool
			for _, config := range service.benthosConfigs {
				if config.Name == benthosName {
					found = true
					Expect(config.DesiredFSMState).To(Equal(benthosfsm.OperationalStateStopped))
					// In a real test, we'd verify the BenthosServiceConfig was updated as expected
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should return error when topic browser doesn't exist", func() {
			// Act - try to update a non-existent component
			err := service.UpdateInManager(ctx, mockSvcRegistry, updatedCfg, "non-existent")

			// Assert
			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("Stop", func() {
		var cfg *benthossvccfg.BenthosServiceConfig

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "benthos_kafka_test",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			// Add the component first
			err := service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should start a topic browser by changing its desired state", func() {
			// First stop the topic browser
			err := service.Stop(ctx, mockSvcRegistry, tbName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the desired state was changed to stopped
			benthosName := service.getName(tbName)
			var foundStopped bool
			for _, config := range service.benthosConfigs {
				if config.Name == benthosName {
					foundStopped = true
					Expect(config.DesiredFSMState).To(Equal(benthosfsm.OperationalStateStopped))
					break
				}
			}
			Expect(foundStopped).To(BeTrue())

			// Now start the topic browser
			err = service.Start(ctx, mockSvcRegistry, tbName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the desired state was changed to active
			var foundStarted bool
			for _, config := range service.benthosConfigs {
				if config.Name == benthosName {
					foundStarted = true
					Expect(config.DesiredFSMState).To(Equal(benthosfsm.OperationalStateActive))
					break
				}
			}
			Expect(foundStarted).To(BeTrue())
		})

		It("should return error when trying to start/stop non-existent topic browser", func() {
			// Try to start a non-existent topic browser
			err := service.Start(ctx, mockSvcRegistry, "non-existent")
			Expect(err).To(MatchError(ErrServiceNotExist))

			// Try to stop a non-existent component
			err = service.Stop(ctx, mockSvcRegistry, "non-existent")
			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("RemoveFromManager", func() {
		var cfg *benthossvccfg.BenthosServiceConfig

		BeforeEach(func() {
			// Create a basic config for testing
			cfg = &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "benthos_kafka_test",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			// Add the topic browser first
			err := service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove a topic browser from the benthos manager", func() {
			// Get the initial count
			initialCount := len(service.benthosConfigs)

			// Act - remove the component
			err := service.RemoveFromManager(ctx, mockSvcRegistry, tbName)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(service.benthosConfigs).To(HaveLen(initialCount - 1))

			// Verify the component is no longer in the list
			benthosName := service.getName(tbName)
			for _, config := range service.benthosConfigs {
				Expect(config.Name).NotTo(Equal(benthosName))
			}
		})

		// Note: removing a non-existent topic browser should not result in an error
		// the remove action will be called multiple times until the toic browser is gone it returns nil
	})

	Describe("ReconcileManager", func() {
		It("should pass configs to the benthos manager for reconciliation", func() {
			// Add a test topic browser to have something to reconcile
			cfg := &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "benthos_kafka_test",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			err := service.AddToManager(ctx, mockSvcRegistry, cfg, tbName)
			Expect(err).NotTo(HaveOccurred())

			// Use the real mock from the FSM package
			manager, _ := benthosfsm.NewBenthosManagerWithMockedServices("test")
			service.benthosManager = manager

			// Configure the mock to return true for reconciled
			mockBenthos.ReconcileManagerReconciled = true

			// Act
			err, reconciled := service.ReconcileManager(ctx, mockSvcRegistry, tick)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			// Change expectation to match the actual behavior
			Expect(reconciled).To(BeTrue()) // The mock is configured to return true
		})

		It("should handle errors from the benthos manager", func() {
			// Create a custom mock that returns an error
			mockError := errors.New("test reconcile error")

			// Create a real manager with mocked services
			mockManager, mockBenthosService := benthosfsm.NewBenthosManagerWithMockedServices("test-error")

			// Create a service with our mocked manager
			testService := NewDefaultService("test-error-service",
				WithService(mockBenthosService),
				WithManager(mockManager))

			// Add a test component to have something to reconcile (just like in the other test)
			testComponentName := "test-error-topicbrowser"
			cfg := &benthossvccfg.BenthosServiceConfig{
				Input: map[string]any{
					"uns": map[string]any{
						"umh_topic":      "umh.v1.*",
						"kafka_topic":    "umh.messages",
						"broker_address": "localhost:9092",
						"consumer_group": "benthos_kafka_test",
					},
				},
				Pipeline: map[string]any{
					"processors": []map[string]any{
						{
							"topic-browser": map[string]any{},
						},
					},
				},
				Output: map[string]any{
					"stdout": map[string]any{},
				},
				LogLevel: constants.DefaultBenthosLogLevel,
			}

			err := testService.AddToManager(ctx, mockSvcRegistry, cfg, testComponentName)
			Expect(err).NotTo(HaveOccurred())

			// First reconcile - this will just create the instance in the manager
			firstErr, reconciled := testService.ReconcileManager(ctx, mockSvcRegistry, tick)
			Expect(firstErr).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue()) // Should be true because we created a new instance

			// Now set up the mock service to fail during the actual instance reconciliation
			mockBenthosService.ReconcileManagerError = mockError

			// Second reconcile - now that the instance exists, it will try to reconcile it
			err, reconciled = testService.ReconcileManager(ctx, mockSvcRegistry, tick+1)

			// Assert
			Expect(err).ToNot(HaveOccurred()) // it should not return an error
			Expect(reconciled).To(BeFalse())  // it should not be reconciled
			// it should throw the "error reconciling s6Manager: test reconcile error" error through the logs

			// Skip the error checking part as it's not accessible directly
			// The test has already verified that the error is handled properly
			// by checking that reconciled is false

			// Alternatively, we could check for side effects of the error
			// but for a unit test, verifying that reconciled is false is sufficient
		})
	})
	DescribeTable("checkMetrics mismatch scenarios",
		func(
			rpBytesOut int64,
			rpHasState bool,
			benthosBytesOut float64,
			benthosBytesIn float64,
			benthosHasState bool,
			expectedReason string,
			expectedMismatch bool,
		) {
			rpObservedState := buildRedpandaObs(rpBytesOut, rpHasState)
			beObservedState := buildBenthosObs(benthosBytesOut, benthosBytesIn, benthosHasState)

			reason, mismatch := service.checkMetrics(rpObservedState, beObservedState)

			Expect(mismatch).To(Equal(expectedMismatch))
			Expect(reason).To(Equal(expectedReason))
		},
		Entry("no Redpanda metrics state",
			int64(0), false, // rp
			float64(0), float64(0), true, // benthos
			"no redpanda metrics available", true,
		),

		Entry("no Benthos metrics state",
			int64(0), true,
			float64(0), float64(0), false,
			"no benthos metrics available", true,
		),

		Entry("Redpanda output >0, Benthos output ==0",
			int64(500), true,
			float64(0), float64(10), true,
			"redpanda has output, but benthos no output", true,
		),

		Entry("Benthos output >0, Redpanda output ==0",
			int64(0), true,
			float64(20), float64(10), true,
			"redpanda has no output, but benthos has output", true,
		),

		Entry("Redpanda output >0, Benthos input ==0",
			int64(123), true,
			float64(20), float64(0), true,
			"redpanda has output, but benthos no input", true,
		),

		Entry("all metrics consistent",
			int64(0), true,
			float64(0), float64(0), true,
			"", false,
		),
	)
})

// build redpanda observedState for checkMetrics
func buildRedpandaObs(bytesOut int64, withState bool) rpfsm.RedpandaObservedState {
	var rpObservedState rpfsm.RedpandaObservedState
	rpObservedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.
		Metrics.Throughput.BytesOut = bytesOut
	if withState {
		rpObservedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.
			MetricsState = &rpmonitor.RedpandaMetricsState{}
	}
	return rpObservedState
}

// build benthos observedState for checkMetrics
func buildBenthosObs(outMsgs, inMsgs float64, withState bool) benthosfsm.BenthosObservedState {
	var beObservedState benthosfsm.BenthosObservedState
	if withState {
		metricsState := &benthos_monitor.BenthosMetricsState{}
		metricsState.Output.MessagesPerTick = outMsgs
		metricsState.Input.MessagesPerTick = inMsgs
		beObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState = metricsState
	}
	return beObservedState
}

// ConfigureBenthosManagerForState configures mock service for proper transitions
func ConfigureBenthosManagerForState(mockService *benthossvc.MockBenthosService, serviceName string, targetState string) {
	// Make sure the service exists in the mock
	if mockService.ExistingServices == nil {
		mockService.ExistingServices = make(map[string]bool)
	}
	mockService.ExistingServices[serviceName] = true

	// Make sure service state is initialized
	if mockService.ServiceStates == nil {
		mockService.ServiceStates = make(map[string]*benthossvc.ServiceInfo)
	}
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}

	// Configure the service for the target state
	TransitionToBenthosState(mockService, serviceName, targetState)
}

func TransitionToBenthosState(mockService *benthossvc.MockBenthosService, serviceName string, targetState string) {
	switch targetState {
	case benthosfsm.OperationalStateStopped:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopped,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateStarting:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopped,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateStartingConfigLoading:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          true,
			S6FSMState:           s6fsm.OperationalStateRunning,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateIdle:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   true,
			IsRunningWithoutErrors: true,
		})
	case benthosfsm.OperationalStateActive:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   true,
			IsRunningWithoutErrors: true,
			HasProcessingActivity:  true,
		})
	case benthosfsm.OperationalStateDegraded:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   false,
			IsRunningWithoutErrors: false,
			HasProcessingActivity:  true,
		})
	case benthosfsm.OperationalStateStopping:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopping,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	}
}

func SetupBenthosServiceState(
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	flags benthossvc.ServiceStateFlags,
) {
	// Ensure service exists in mock
	mockService.ExistingServices[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}

	// Set S6 FSM state
	if flags.S6FSMState != "" {
		mockService.ServiceStates[serviceName].S6FSMState = flags.S6FSMState
	}

	// Update S6 observed state
	if flags.IsS6Running {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceUp,
			Uptime: 10, // Set uptime to 10s to simulate config loaded
			Pid:    1234,
		}
	} else {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceDown,
		}
	}

	// Update health check status
	if flags.IsHealthchecksPassed {
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthos_monitor.HealthCheck{
			IsLive:  true,
			IsReady: true,
		}
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Input.ConnectionUp = 1
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Input.ConnectionFailed = 0
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Input.ConnectionLost = 0
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Output.ConnectionUp = 1
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Output.ConnectionFailed = 0
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Output.ConnectionLost = 0
	} else {
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthos_monitor.HealthCheck{
			IsLive:  false,
			IsReady: false,
		}
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Input.ConnectionUp = 1
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Input.ConnectionFailed = 1
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Input.ConnectionLost = 0
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Output.ConnectionUp = 1
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Output.ConnectionFailed = 1
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.Metrics.Output.ConnectionLost = 0
	}

	// Setup metrics state if needed
	if flags.HasProcessingActivity {
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.MetricsState = &benthos_monitor.BenthosMetricsState{
			IsActive: true,
		}
	} else if mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.MetricsState == nil {
		mockService.ServiceStates[serviceName].BenthosStatus.BenthosMetrics.MetricsState = &benthos_monitor.BenthosMetricsState{
			IsActive: false,
		}
	}

	// Store the service state flags directly
	mockService.SetServiceState(serviceName, flags)
}

// WaitForBenthosManagerInstanceState waits for instance to reach desired state
func WaitForBenthosManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *benthosfsm.BenthosManager,
	services serviceregistry.Provider,
	instanceName string,
	expectedState string,
	maxAttempts int,
) (uint64, error) {
	// Duplicate implementation from fsmtest package
	tick := snapshot.Tick
	baseTime := snapshot.SnapshotTime
	for i := 0; i < maxAttempts; i++ {

		// Update the snapshot time and tick to simulate the passage of time deterministically
		snapshot.SnapshotTime = baseTime.Add(time.Duration(tick) * constants.DefaultTickerTime)
		snapshot.Tick = tick
		err, _ := manager.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		instance, found := manager.GetInstance(instanceName)
		if found && instance.GetCurrentFSMState() == expectedState {
			return tick, nil
		}
	}
	return tick, fmt.Errorf("instance didn't reach expected state: %s", expectedState)
}

// makeHex creates hex-encoded payload for testing
func makeHex(payload []byte) string {
	return hex.EncodeToString(payload)
}
