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

package redpanda_test

import (
	"context"
	"errors"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	redpandaserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	redpanda_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RedpandaInstance getServiceStatus", func() {
	var (
		instance        *redpanda.RedpandaInstance
		mockService     *redpanda_service.MockRedpandaService
		mockSvcRegistry serviceregistry.Provider
		ctx             context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockSvcRegistry = serviceregistry.NewMockRegistry()

		// Create a new RedpandaInstance
		cfg := config.RedpandaConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name: "test-redpanda",
			},
			RedpandaServiceConfig: redpandaserviceconfig.RedpandaServiceConfig{},
		}
		instance = redpanda.NewRedpandaInstance(cfg)

		// Create and inject a mock service
		mockService = redpanda_service.NewMockRedpandaService()

		// Set up a default ServiceInfo with failed health checks that our error handling can modify
		mockService.ServiceState = &redpanda_service.ServiceInfo{
			RedpandaStatus: redpanda_service.RedpandaStatus{
				HealthCheck: redpanda_service.HealthCheck{
					IsLive:  false,
					IsReady: false,
				},
			},
		}

		// Mark the service as existing by default
		mockService.ServiceExistsFlag = true

		instance.SetService(mockService)
	})

	Describe("When handling ErrServiceNotExist", func() {
		BeforeEach(func() {
			// Setup mock to return ErrServiceNotExist
			mockService.StatusError = redpanda_service.ErrServiceNotExist
			// Also mark the service as not existing
			mockService.ServiceExistsFlag = false
		})

		It("should return the error in LifecycleStateToBeCreated", func() {
			instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(internalfsm.LifecycleStateToBeCreated)
			_, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
			Expect(err).To(Equal(redpanda_service.ErrServiceNotExist))
		})

		It("should return the error in LifecycleStateCreating", func() {
			instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(internalfsm.LifecycleStateCreating)
			_, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
			Expect(err).To(Equal(redpanda_service.ErrServiceNotExist))
		})

		It("should not return error in LifecycleStateRemoving", func() {
			instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(internalfsm.LifecycleStateRemoving)
			_, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
			Expect(err).To(BeNil())
		})

		It("should not return error in LifecycleStateRemoved", func() {
			instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(internalfsm.LifecycleStateRemoved)
			_, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
			Expect(err).To(BeNil())
		})

		It("should not return error in operational states", func() {
			states := []string{
				redpanda.OperationalStateStopped,
				redpanda.OperationalStateStarting,
				redpanda.OperationalStateIdle,
				redpanda.OperationalStateActive,
				redpanda.OperationalStateDegraded,
				redpanda.OperationalStateStopping,
			}

			for _, state := range states {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				_, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(BeNil())
			}
		})
	})

	Describe("When handling ErrServiceNoLogFile", func() {
		BeforeEach(func() {
			// Setup mock to return ErrServiceNoLogFile
			mockService.StatusError = redpanda_service.ErrServiceNoLogFile

			// For testing health check failure in Idle/Active states, we need a ServiceInfo
			// that has the right properties
			mockService.ServiceState = &redpanda_service.ServiceInfo{
				RedpandaStatus: redpanda_service.RedpandaStatus{
					HealthCheck: redpanda_service.HealthCheck{
						IsLive:  false,
						IsReady: false,
					},
				},
			}
		})

		It("should return ServiceInfo with failed health checks in Active state", func() {
			instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(redpanda.OperationalStateActive)
			info, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
			Expect(err).To(BeNil())
			Expect(info.RedpandaStatus.HealthCheck.IsLive).To(BeFalse())
			Expect(info.RedpandaStatus.HealthCheck.IsReady).To(BeFalse())
		})

		It("should return ServiceInfo with failed health checks in Idle state", func() {
			instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(redpanda.OperationalStateIdle)
			info, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
			Expect(err).To(BeNil())
			Expect(info.RedpandaStatus.HealthCheck.IsLive).To(BeFalse())
			Expect(info.RedpandaStatus.HealthCheck.IsReady).To(BeFalse())
		})

		It("should return nil error in lifecycle states", func() {
			states := []string{
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				internalfsm.LifecycleStateRemoving,
				internalfsm.LifecycleStateRemoved,
			}

			for _, state := range states {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				info, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(BeNil())
				// ServiceInfo should be returned as-is in these states
				Expect(info).To(Equal(redpanda_service.ServiceInfo{}))
			}
		})

		It("should return nil error in non-running states", func() {
			states := []string{
				redpanda.OperationalStateStopped,
				redpanda.OperationalStateStarting,
				redpanda.OperationalStateStopping,
			}

			for _, state := range states {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				info, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(BeNil())
				// ServiceInfo should be returned as-is in these states
				Expect(info).To(Equal(redpanda_service.ServiceInfo{}))
			}
		})
	})

	Describe("When handling ErrServiceConnectionRefused", func() {
		BeforeEach(func() {
			// Setup mock to return ErrServiceConnectionRefused
			mockService.StatusError = monitor.ErrServiceConnectionRefused

			// For testing health check failure in running states, we need a ServiceInfo
			// that has the right properties
			mockService.ServiceState = &redpanda_service.ServiceInfo{
				RedpandaStatus: redpanda_service.RedpandaStatus{
					HealthCheck: redpanda_service.HealthCheck{
						IsLive:  false,
						IsReady: false,
					},
				},
			}
		})

		It("should return ServiceInfo with failed health checks in lifecycle states", func() {
			lifecycleStates := []string{
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				internalfsm.LifecycleStateRemoving,
				internalfsm.LifecycleStateRemoved,
			}

			for _, state := range lifecycleStates {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				info, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(BeNil())
				Expect(info.RedpandaStatus.HealthCheck.IsLive).To(BeFalse())
				Expect(info.RedpandaStatus.HealthCheck.IsReady).To(BeFalse())
			}
		})

		It("should return ServiceInfo with failed health checks in non-running states", func() {
			nonRunningStates := []string{
				redpanda.OperationalStateStopped,
				redpanda.OperationalStateStarting,
				redpanda.OperationalStateStopping,
			}

			for _, state := range nonRunningStates {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				info, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(BeNil())
				Expect(info.RedpandaStatus.HealthCheck.IsLive).To(BeFalse())
				Expect(info.RedpandaStatus.HealthCheck.IsReady).To(BeFalse())
			}
		})

		It("should return error in running states", func() {
			runningStates := []string{
				redpanda.OperationalStateIdle,
				redpanda.OperationalStateActive,
				redpanda.OperationalStateDegraded,
			}

			for _, state := range runningStates {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				_, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(Equal(monitor.ErrServiceConnectionRefused))
			}
		})
	})

	Describe("When handling other errors", func() {
		BeforeEach(func() {
			// Setup mock to return a generic error
			mockService.StatusError = errors.New("generic error")
		})

		It("should return the original error in all states", func() {
			states := []string{
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				internalfsm.LifecycleStateRemoving,
				internalfsm.LifecycleStateRemoved,
				redpanda.OperationalStateStopped,
				redpanda.OperationalStateStarting,
				redpanda.OperationalStateIdle,
				redpanda.OperationalStateActive,
				redpanda.OperationalStateDegraded,
				redpanda.OperationalStateStopping,
			}

			for _, state := range states {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				_, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(MatchError("generic error"))
			}
		})
	})

	Describe("When service is available", func() {
		BeforeEach(func() {
			// Setup mock to return success
			mockService.StatusError = nil
			mockService.ServiceState = &redpanda_service.ServiceInfo{
				RedpandaStatus: redpanda_service.RedpandaStatus{
					HealthCheck: redpanda_service.HealthCheck{
						IsLive:  true,
						IsReady: true,
					},
				},
			}
		})

		It("should return the service info without error in all states", func() {
			states := []string{
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
				internalfsm.LifecycleStateRemoving,
				internalfsm.LifecycleStateRemoved,
				redpanda.OperationalStateStopped,
				redpanda.OperationalStateStarting,
				redpanda.OperationalStateIdle,
				redpanda.OperationalStateActive,
				redpanda.OperationalStateDegraded,
				redpanda.OperationalStateStopping,
			}

			for _, state := range states {
				instance.GetBaseFSMInstanceForTest().SetCurrentFSMState(state)
				info, err := instance.GetServiceStatus(ctx, mockSvcRegistry, 1, time.Now())
				Expect(err).To(BeNil())
				Expect(info.RedpandaStatus.HealthCheck.IsLive).To(BeTrue())
				Expect(info.RedpandaStatus.HealthCheck.IsReady).To(BeTrue())
			}
		})
	})
})
