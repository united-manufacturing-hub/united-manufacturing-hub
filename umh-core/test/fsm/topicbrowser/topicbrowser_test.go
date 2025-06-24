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

//go:build test
// +build test

package topicbrowser_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	tbsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("TopicBrowser FSM", func() {
	var (
		instance        *topicbrowser.Instance
		mockService     *tbsvc.MockTopicBrowserService
		serviceName     string
		ctx             context.Context
		tick            uint64
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx = context.Background()
		tick = 0

		// Create default instance with desired state of "stopped" initially
		serviceName = "test-topicbrowser"
		inst, ms := fsmtest.SetupTopicBrowserInstance(serviceName, topicbrowser.OperationalStateStopped)
		instance = inst
		mockService = ms
		mockSvcRegistry = serviceregistry.NewMockRegistry()
	})

	// -------------------------------------------------------------------------
	//  BASIC STATE TRANSITIONS
	// -------------------------------------------------------------------------
	Context("Basic State Transitions", func() {
		It("should transition from ToBeCreated to Creating to Stopped", func() {
			var err error

			// from "to_be_created" => "creating"
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				5, // attempts
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// Mock the service creation success
			mockService.ServiceStates[serviceName] = &tbsvc.ServiceInfo{
				S6FSMState: s6fsm.OperationalStateStopped,
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{Status: s6svc.ServiceDown, Uptime: 5},
				},
				BenthosObservedState: benthosfsm.BenthosObservedState{
					ServiceInfo: benthos_monitor.ServiceInfo{
						BenthosStatus: benthos_monitor.BenthosStatus{
							BenthosMetrics: benthos_monitor.BenthosMetrics{
								MetricsState: &benthos_monitor.BenthosMetricsState{
									IsActive: false,
								},
							},
						},
					},
				},
			}
			mockService.ExistingServices[serviceName] = true

			// from "creating" => "stopped"
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Stopped to Starting when activated", func() {
			var err error

			// Setup instance in stopped state first
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStopped, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set desired state = Active => from "stopped" => "starting"
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(topicbrowser.OperationalStateActive))

			// Set mock flags for initial start attempt
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running: false,
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Check that StartTopicBrowser was called
			Expect(mockService.StartCalled).To(BeTrue())
		})

		It("should transition from Starting to Active when all conditions are met", func() {
			var err error

			// Setup instance in starting state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStarting, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set all conditions for successful start
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running:           true,
				S6FSMState:            s6fsm.OperationalStateRunning,
				IsHealthchecksPassed:  true,
				IsRunningStably:       true,
				HasProcessingActivity: true,
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				10, // More attempts for complex transition
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  DEGRADED STATE HANDLING
	// -------------------------------------------------------------------------
	Context("Degraded State Handling", func() {
		It("should transition to Degraded when S6 service is not running", func() {
			var err error

			// Setup instance in active state first
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Simulate S6 service failure
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running:          false,
				IsHealthchecksPassed: false,
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegraded,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to Degraded when Redpanda has activity but TopicBrowser doesn't", func() {
			var err error

			// Setup instance in active state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Simulate Redpanda activity but no TopicBrowser activity
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running:           true,
				IsHealthchecksPassed:  true,
				HasRedpandaActivity:   true,
				HasProcessingActivity: false, // TopicBrowser has no activity
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegraded,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should recover from Degraded when conditions are resolved", func() {
			var err error

			// Setup instance in degraded state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateDegraded, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Restore healthy conditions
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running:           true,
				IsHealthchecksPassed:  true,
				IsRunningStably:       true,
				HasProcessingActivity: true,
				HasRedpandaActivity:   true,
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateDegraded,
				topicbrowser.OperationalStateIdle, // Recovers to idle first
				10,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  IDLE/ACTIVE TRANSITIONS
	// -------------------------------------------------------------------------
	Context("Idle/Active State Transitions", func() {
		It("should transition from Active to Idle when no processing activity", func() {
			var err error

			// Setup instance in active state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Remove processing activity
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running:           true,
				IsHealthchecksPassed:  true,
				IsRunningStably:       true,
				HasProcessingActivity: false, // No activity
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateIdle,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Idle to Active when processing activity resumes", func() {
			var err error

			// Setup instance in idle state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateIdle, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Restore processing activity
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running:           true,
				IsHealthchecksPassed:  true,
				IsRunningStably:       true,
				HasProcessingActivity: true, // Activity resumed
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateIdle,
				topicbrowser.OperationalStateActive,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  STOPPING STATES
	// -------------------------------------------------------------------------
	Context("Stopping States", func() {
		It("should transition to Stopping when desired state is Stopped", func() {
			var err error

			// Setup instance in active state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set desired state to stopped
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateStopped)).To(Succeed())

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateStopping,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Check that StopTopicBrowser was called
			Expect(mockService.StopCalled).To(BeTrue())
		})

		It("should reach Stopped state when service is fully stopped", func() {
			var err error

			// Setup instance in stopping state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStopping, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Simulate service fully stopped
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running: false,
				IsS6Stopped: true,
				S6FSMState:  s6fsm.OperationalStateStopped,
			})

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStopping,
				topicbrowser.OperationalStateStopped,
				5,
				tick,
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING AND RECOVERY
	// -------------------------------------------------------------------------
	Context("Error Handling and Recovery", func() {
		It("should handle service creation errors with backoff", func() {
			var err error

			// Simulate creation error
			mockService.AddToManagerError = tbsvc.ErrServiceCreationFailed

			// Attempt transition should fail and set backoff
			_, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				1, // Single attempt - should fail
				tick,
				time.Now(),
			)
			Expect(err).To(HaveOccurred())

			// Verify backoff is set
			Expect(instance.GetBackoffError(tick + 1)).NotTo(BeNil())
		})

		It("should handle transient failures and retry", func() {
			var err error

			// Setup instance in starting state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStarting, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Simulate transient start failure
			mockService.StartError = tbsvc.ErrTransientFailure

			// First attempt should fail
			_, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				1,
				tick,
				time.Now(),
			)
			Expect(err).To(HaveOccurred())

			// Clear error and try again
			mockService.StartError = nil
			mockService.SetServiceState(serviceName, tbsvc.ServiceStateFlags{
				IsS6Running:          true,
				IsHealthchecksPassed: true,
				IsRunningStably:      true,
			})

			// Should succeed after error is cleared
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				5,
				tick+10, // Advance tick to clear backoff
				time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  CONFIGURATION MANAGEMENT
	// -------------------------------------------------------------------------
	Context("Configuration Management", func() {
		It("should detect and handle configuration changes", func() {
			var err error

			// Setup instance in active state
			tick, err = fsmtest.SetupTopicBrowserInState(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				topicbrowser.OperationalStateActive, tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Simulate configuration change
			newConfig := topicbrowserserviceconfig.Config{
				// Modified config fields
			}
			mockService.ObservedConfigs[serviceName] = newConfig

			// Should trigger config update
			tick, err = fsmtest.StabilizeTopicBrowserInstance(
				ctx, instance, mockService, mockSvcRegistry, serviceName,
				5, tick, time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.UpdateInManagerCalled).To(BeTrue())
		})
	})
})

// Helper functions for tests
func createMockSystemSnapshot() fsm.SystemSnapshot {
	return fsm.SystemSnapshot{
		Tick:         1,
		SnapshotTime: time.Now(),
		Managers:     make(map[string]fsm.ManagerInterface),
	}
}
