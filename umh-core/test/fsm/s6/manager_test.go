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

package s6_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("S6Manager", func() {
	var (
		manager *s6fsm.S6Manager
		ctx     context.Context
		tick    uint64
		cancel  context.CancelFunc
		mockFS  filesystem.Service
	)

	AfterEach(func() {
		cancel()
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second) // we need to have a deadline as the reconcile logic in the base fsm manager requires it
		tick = 0

		manager = s6fsm.NewS6ManagerWithMockedServices("")
		mockFS = filesystem.NewMockFileSystem()
	})

	Context("Initialization", func() {
		It("should handle empty config without errors", func() {
			// Create empty config
			emptyConfig := []config.S6FSMConfig{}

			// Reconcile with empty config using a single reconciliation
			err, _ := manager.Reconcile(ctx, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: emptyConfig}}, Tick: tick}, mockFS)
			tick++
			Expect(err).NotTo(HaveOccurred())

			// Verify that no instances were created
			Expect(manager.GetInstances()).To(BeEmpty())
		})

		It("should initialize service in stopped state and maintain it", func() {
			// First setup with empty config to ensure no instances exist
			emptyConfig := []config.S6FSMConfig{}
			err, _ := manager.Reconcile(ctx, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: emptyConfig}}, Tick: tick}, mockFS)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())

			// Create a service name for testing
			serviceName := "test-stopped-service"

			// Create config with explicitly desired stopped state
			configWithStoppedService := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo hello"},
						Env:         map[string]string{"TEST_ENV": "test_value"},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Reconcile to create service and ensure it reaches stopped state
			var nextTick uint64
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: configWithStoppedService}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Verify service properties
			Expect(manager.GetInstances()).To(HaveLen(1))
			Expect(manager.GetInstances()).To(HaveKey(serviceName))

			service, ok := manager.GetInstance(serviceName)
			Expect(ok).To(BeTrue())

			// Verify state is stopped
			Expect(service.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
			Expect(service.GetDesiredFSMState()).To(Equal(s6fsm.OperationalStateStopped))

			// Verify state remains stable over multiple reconciliation cycles
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: configWithStoppedService}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 3)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Double-check the final state
			Expect(service.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
			Expect(service.GetDesiredFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should initialize service in running state when config requests it", func() {
			// First setup with empty config to ensure no instances exist
			emptyConfig := []config.S6FSMConfig{}
			err, _ := manager.Reconcile(ctx, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: emptyConfig}}, Tick: tick}, mockFS)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(BeEmpty())

			// Create a service name for testing
			serviceName := "test-running-service"

			// Create config with explicitly desired running state
			configWithRunningService := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateRunning,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo hello"},
						Env:         map[string]string{"TEST_ENV": "test_value"},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Reconcile to create service and wait for it to reach running state
			var nextTick uint64
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: configWithRunningService}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateRunning, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Verify service properties
			Expect(manager.GetInstances()).To(HaveLen(1))
			Expect(manager.GetInstances()).To(HaveKey(serviceName))

			service, ok := manager.GetInstance(serviceName)
			Expect(ok).To(BeTrue())

			// Verify state is running
			Expect(service.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))
			Expect(service.GetDesiredFSMState()).To(Equal(s6fsm.OperationalStateRunning))
		})
	})

	Context("State Transitions", func() {
		It("should change instance state from stopped to running when config changes", func() {
			// Start with a service in stopped state
			serviceName := "transition-test-service"
			initialConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo test-service"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Create and wait for the service to reach stopped state
			var err error
			var nextTick uint64
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: initialConfig}}, Tick: tick}, mockFS,
				serviceName, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create instance")

			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: initialConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to reach stopped state")

			// Verify service is in stopped state
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
			Expect(instance.GetDesiredFSMState()).To(Equal(s6fsm.OperationalStateStopped))

			// Update config to change desired state to running
			updatedConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateRunning,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo test-service"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Wait for service to transition to running state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: updatedConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateRunning, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to transition to running state")

			// Verify service is now in running state
			instance, exists = manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))
			Expect(instance.GetDesiredFSMState()).To(Equal(s6fsm.OperationalStateRunning))

			// Now change back to stopped
			stoppedConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo test-service"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Wait for service to transition back to stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: stoppedConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to transition back to stopped state")

			// Verify service is back in stopped state
			instance, exists = manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
			Expect(instance.GetDesiredFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})
	})

	Context("Lifecycle", func() {
		It("should handle full lifecycle from creation to running to removal", func() {
			// Create service in running state
			serviceName := "lifecycle-test-service"
			runningConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateRunning,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo lifecycle-test"},
						Env:         map[string]string{"LIFECYCLE": "test"},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Wait for instance to be created
			var err error
			var nextTick uint64
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: runningConfig}}, Tick: tick}, mockFS,
				serviceName, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create instance")

			// Wait for service to reach running state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: runningConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateRunning, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to reach running state")

			// Verify instance is running
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))

			// First change to stopped - this is cleaner than removing directly from running
			stoppedConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo lifecycle-test"},
						Env:         map[string]string{"LIFECYCLE": "test"},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Wait for service to reach stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: stoppedConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to transition to stopped state")

			// Now remove the service from config
			emptyConfig := []config.S6FSMConfig{}

			// Reconcile with empty config and wait for the service to be removed
			// Using more attempts since removal might take longer
			nextTick, err = fsmtest.WaitForManagerInstanceRemoval(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: emptyConfig}}, Tick: tick}, mockFS,
				serviceName, 25)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to remove instance")

			// Verify instance has been removed
			_, exists = manager.GetInstance(serviceName)
			Expect(exists).To(BeFalse(), "Instance should have been removed")
			Expect(manager.GetInstances()).To(BeEmpty())
		})
	})

	Context("Updates", func() {
		It("should create a new instance when a service is added to existing config", func() {
			// Start with one service in the config
			initialServiceName := "initial-service"
			initialConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            initialServiceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo hello"},
						Env:         map[string]string{"TEST_ENV": "test_value"},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Wait for initial service to be created and reach stopped state
			var err error
			var nextTick uint64
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: initialConfig}}, Tick: tick}, mockFS,
				initialServiceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetInstances()).To(HaveLen(1))

			// Add a new service to the config
			newServiceName := "new-service"
			updatedConfig := append(initialConfig, config.S6FSMConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            newServiceName,
					DesiredFSMState: s6fsm.OperationalStateStopped,
				},
				S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
					Command:     []string{"/bin/sh", "-c", "echo world"},
					Env:         map[string]string{"NEW_ENV": "new_value"},
					ConfigFiles: map[string]string{},
				},
			})

			// Wait for the new instance to be created
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: updatedConfig}}, Tick: tick}, mockFS,
				newServiceName, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create new instance")

			// Now wait for the new service to reach stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: updatedConfig}}, Tick: tick}, mockFS,
				newServiceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Verify both services exist and are in the correct state
			Expect(manager.GetInstances()).To(HaveLen(2))
			Expect(manager.GetInstances()).To(HaveKey(initialServiceName))
			Expect(manager.GetInstances()).To(HaveKey(newServiceName))

			initialService, ok := manager.GetInstance(initialServiceName)
			Expect(ok).To(BeTrue())
			Expect(initialService.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))

			newService, ok := manager.GetInstance(newServiceName)
			Expect(ok).To(BeTrue())
			Expect(newService.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should stop and remove an instance when a service is removed from config", func() {
			// Start with two services in the config
			service1Name := "service-to-keep"
			service2Name := "service-to-remove"
			initialConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            service1Name,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo service1"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            service2Name,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo service2"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// First reconcile with initial config to create both services
			var err error
			var nextTick uint64

			// Wait for first service to be created
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: initialConfig}}, Tick: tick}, mockFS,
				service1Name, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create first instance")

			// Wait for second service to be created
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: initialConfig}}, Tick: tick}, mockFS,
				service2Name, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create second instance")

			// Wait for first service to reach stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: initialConfig}}, Tick: tick}, mockFS,
				service1Name, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Wait for second service to reach stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: initialConfig}}, Tick: tick}, mockFS,
				service2Name, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Verify both services exist
			Expect(manager.GetInstances()).To(HaveLen(2))
			Expect(manager.GetInstances()).To(HaveKey(service1Name))
			Expect(manager.GetInstances()).To(HaveKey(service2Name))

			// Update config to remove the second service
			updatedConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            service1Name,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo service1"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Wait for the service to be completely removed from the manager
			nextTick, err = fsmtest.WaitForManagerInstanceRemoval(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: updatedConfig}}, Tick: tick}, mockFS,
				service2Name, 15)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed waiting for instance removal")

			// Verify that only the first service remains
			Expect(manager.GetInstances()).To(HaveLen(1))
			Expect(manager.GetInstances()).To(HaveKey(service1Name))
			Expect(manager.GetInstances()).NotTo(HaveKey(service2Name))

			// Verify the remaining service is still in the correct state
			remainingService, ok := manager.GetInstance(service1Name)
			Expect(ok).To(BeTrue())
			Expect(remainingService.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})
	})

	Context("Error Handling", func() {
		It("should recover instance after transient errors", func() {
			// Create a service for testing
			serviceName := "test-transient-error"
			serviceConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo test-service"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Create and wait for the service to reach stopped state
			var err error
			var nextTick uint64
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: serviceConfig}}, Tick: tick}, mockFS,
				serviceName, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create instance")

			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: serviceConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to reach stopped state")

			// Get the instance and its mock service
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			s6Instance, ok := instance.(*s6fsm.S6Instance)
			Expect(ok).To(BeTrue())

			mockService, ok := s6Instance.GetService().(*s6service.MockService)
			Expect(ok).To(BeTrue(), "Service is not a mock service")

			// Configure the mock service to fail temporarily
			mockService.StatusError = fmt.Errorf("temporary error fetching service state")

			// Run reconciliation a few times with the error active
			nextTick, err = fsmtest.RunMultipleReconciliations(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: serviceConfig}}, Tick: tick}, mockFS, 10)
			tick = nextTick

			// Verify the instance still exists despite errors
			_, exists = manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue(), "Instance should still exist despite errors")

			// Remove the error
			mockService.StatusError = nil

			// Wait for the instance to recover to stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: serviceConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to recover to stopped state")

			// Verify the instance is in expected state after recovery
			instance, exists = manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should remove an instance when it encounters a permanent error in a terminal state", func() {
			// Create a service for testing
			serviceName := "test-permanent-error"
			serviceConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            serviceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo test-service"},
						Env:         map[string]string{},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Create and wait for the service to reach stopped state
			var err error
			var nextTick uint64
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: serviceConfig}}, Tick: tick}, mockFS,
				serviceName, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create instance")

			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: serviceConfig}}, Tick: tick}, mockFS,
				serviceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to reach stopped state")

			// Get the instance and its mock service
			instance, exists := manager.GetInstance(serviceName)
			Expect(exists).To(BeTrue())
			s6Instance, ok := instance.(*s6fsm.S6Instance)
			Expect(ok).To(BeTrue())

			mockService, ok := s6Instance.GetService().(*s6service.MockService)
			Expect(ok).To(BeTrue(), "Service is not a mock service")

			// Configure the mock service to fail with a permanent error
			permanentErrorMsg := backoff.PermanentFailureError + ": critical configuration error"
			mockService.GetConfigError = fmt.Errorf("%s", permanentErrorMsg)

			// Run several reconciliations to allow the error to be detected and handled
			nextTick, err = fsmtest.RunMultipleReconciliations(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: serviceConfig}}, Tick: tick}, mockFS, 5)
			tick = nextTick

			// Verify the instance has been removed due to permanent error
			_, exists = manager.GetInstance(serviceName)
			Expect(exists).To(BeFalse(), "Instance should be removed after permanent error")
		})
	})

	Context("Multiple Instances", func() {
		It("should start two services simultaneously and handle them independently", func() {
			// Create configuration for two services
			service1Name := "multi-test-service1"
			service2Name := "multi-test-service2"

			multiServiceConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            service1Name,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo service1"},
						Env:         map[string]string{"SERVICE": "1"},
						ConfigFiles: map[string]string{},
					},
				},
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            service2Name,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo service2"},
						Env:         map[string]string{"SERVICE": "2"},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Reconcile to create both services
			var err error
			var nextTick uint64

			// Wait for both services to be created
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				service1Name, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create first service")

			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				service2Name, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create second service")

			// Wait for both services to reach stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				service1Name, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "First service failed to reach stopped state")

			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				service2Name, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Second service failed to reach stopped state")

			// Verify both services exist and are in stopped state
			Expect(manager.GetInstances()).To(HaveLen(2))
			Expect(manager.GetInstances()).To(HaveKey(service1Name))
			Expect(manager.GetInstances()).To(HaveKey(service2Name))

			service1, ok := manager.GetInstance(service1Name)
			Expect(ok).To(BeTrue())
			Expect(service1.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))

			service2, ok := manager.GetInstance(service2Name)
			Expect(ok).To(BeTrue())
			Expect(service2.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should handle one service failing while the other remains stable", func() {
			// Create configuration for two services
			stableServiceName := "stable-service"
			failingServiceName := "failing-service"

			multiServiceConfig := []config.S6FSMConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            stableServiceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo stable-service"},
						Env:         map[string]string{"TYPE": "stable"},
						ConfigFiles: map[string]string{},
					},
				},
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            failingServiceName,
						DesiredFSMState: s6fsm.OperationalStateStopped,
					},
					S6ServiceConfig: s6serviceconfig.S6ServiceConfig{
						Command:     []string{"/bin/sh", "-c", "echo failing-service"},
						Env:         map[string]string{"TYPE": "failing"},
						ConfigFiles: map[string]string{},
					},
				},
			}

			// Create and wait for both services to reach stopped state
			var err error
			var nextTick uint64

			// Wait for both instances to be created
			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				stableServiceName, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create stable service")

			nextTick, err = fsmtest.WaitForManagerInstanceCreation(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				failingServiceName, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failed to create failing service")

			// Wait for both to reach stopped state
			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				stableServiceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Stable service failed to reach stopped state")

			nextTick, err = fsmtest.WaitForMockedManagerInstanceState(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				failingServiceName, s6fsm.OperationalStateStopped, 20)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred(), "Failing service failed to reach stopped state")

			// Verify both services exist and are in correct state initially
			Expect(manager.GetInstances()).To(HaveLen(2))
			Expect(manager.GetInstances()).To(HaveKey(stableServiceName))
			Expect(manager.GetInstances()).To(HaveKey(failingServiceName))

			// Get the failing instance and its mock service
			failingInstance, exists := manager.GetInstance(failingServiceName)
			Expect(exists).To(BeTrue())
			s6Instance, ok := failingInstance.(*s6fsm.S6Instance)
			Expect(ok).To(BeTrue())

			mockService, ok := s6Instance.GetService().(*s6service.MockService)
			Expect(ok).To(BeTrue(), "Service is not a mock service")

			// Configure permanent error for the failing service
			permanentErrorMsg := backoff.PermanentFailureError + ": critical configuration error"
			mockService.GetConfigError = fmt.Errorf("%s", permanentErrorMsg)

			// Run reconciliation multiple times to allow error handling
			nextTick, err = fsmtest.RunMultipleReconciliations(ctx, manager, fsm.SystemSnapshot{CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: multiServiceConfig}}, Tick: tick}, mockFS,
				5)
			tick = nextTick

			// Verify failing service is removed
			_, exists = manager.GetInstance(failingServiceName)
			Expect(exists).To(BeFalse(), "Failing service should have been removed")

			// Verify stable service still exists and is in correct state
			stableInstance, exists := manager.GetInstance(stableServiceName)
			Expect(exists).To(BeTrue(), "Stable service should still exist")
			Expect(stableInstance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped),
				"Stable service should remain in stopped state")
		})
	})
})
