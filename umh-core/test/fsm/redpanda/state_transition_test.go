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

package redpanda_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	s6 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	redpandaserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("RedpandaMonitor Service State Transitions", func() {
	var (
		mockS6Service   *s6service.MockService
		mockFileSystem  *filesystem.MockFileSystem
		redpandaService *redpanda.RedpandaService
		ctx             context.Context
		cancel          context.CancelFunc
		redpandaConfig  *redpandaserviceconfig.RedpandaServiceConfig
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		mockS6Service = s6service.NewMockService()
		mockFileSystem = filesystem.NewMockFileSystem()

		// Set up mock logs
		mockS6Service.GetLogsResult = createRedpandaMockLogs()

		// Set default state to stopped
		mockS6Service.StatusResult = s6service.ServiceInfo{
			Status: s6service.ServiceDown,
		}

		// Create a mocked S6 manager with mocked services to prevent using real S6 functionality
		mockedS6Manager := s6fsm.NewS6ManagerWithMockedServices("redpanda")

		// Create the service with mocked dependencies
		redpandaService = redpanda.NewDefaultRedpandaService("redpanda",
			redpanda.WithS6Service(mockS6Service),
			redpanda.WithS6Manager(mockedS6Manager),
		)

		// Set up the redpanda config
		redpandaConfig = &redpandaserviceconfig.RedpandaServiceConfig{}
		redpandaConfig.Topic.DefaultTopicRetentionMs = 604800000
		redpandaConfig.Topic.DefaultTopicRetentionBytes = 0

		// Set up what happens when AddRedpandaMonitorToS6Manager is called
		// We need to ensure that the instance created by the manager also uses the mock service
		err := redpandaService.AddRedpandaToS6Manager(ctx, redpandaConfig, mockFileSystem)
		Expect(err).NotTo(HaveOccurred())
		err, reconciled := redpandaService.ReconcileManager(ctx, mockFileSystem, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconciled).To(BeTrue())
		// Get the instance after reconciliation
		if instance, exists := mockedS6Manager.GetInstance("redpanda"); exists {
			// Type assert to S6Instance
			if s6Instance, ok := instance.(*s6fsm.S6Instance); ok {
				// Set our mock service
				s6Instance.SetService(mockS6Service)
			}
		}
	})

	AfterEach(func() {
		cancel()
	})

	Context("Service lifecycle with state transitions", func() {
		It("should transition from stopped to running and remain stable for 60 reconciliation cycles", func() {
			var serviceInfo redpanda.ServiceInfo
			var err error
			tick := uint64(1) // 1 since we already did one reconciliation in the beforeEach

			By("reconciliation should put the service into creating")
			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6.LifecycleStateCreating)
			// Verify initial state
			serviceInfo, err = redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6.LifecycleStateCreating))
			Expect(serviceInfo.RedpandaStatus.HealthCheck.IsLive).To(BeFalse())
			tick++

			By("reconciliation should put the service into stopped")
			tick = reconcileRedpandaUntilState(ctx, redpandaService, mockFileSystem, tick, s6fsm.OperationalStateStopped)
			// Verify state
			serviceInfo, err = redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateStopped))
			Expect(serviceInfo.RedpandaStatus.HealthCheck.IsLive).To(BeFalse())
			tick++

		})
	})
})

func reconcileRedpandaUntilState(ctx context.Context, redpandaService *redpanda.RedpandaService, mockFileSystem *filesystem.MockFileSystem, tick uint64, expectedState string) uint64 {
	for i := 0; i < 10; i++ {
		err, _ := redpandaService.ReconcileManager(ctx, mockFileSystem, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++

		// Check state
		serviceInfo, err := redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
		Expect(err).NotTo(HaveOccurred())
		if serviceInfo.S6FSMState == expectedState {
			return tick
		}
	}

	Fail(fmt.Sprintf("Expected state %s not reached after 10 reconciliations", expectedState))
	return 0
}

func ensureRedpandaState(ctx context.Context, redpandaService *redpanda.RedpandaService, mockFileSystem *filesystem.MockFileSystem, tick uint64, expectedState string, iterations int) {
	for i := 0; i < iterations; i++ {
		err, _ := redpandaService.ReconcileManager(ctx, mockFileSystem, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++

		// Check state
		serviceInfo, err := redpandaService.Status(ctx, mockFileSystem, tick, time.Now())
		Expect(err).NotTo(HaveOccurred())
		Expect(serviceInfo.S6FSMState).To(Equal(expectedState))
	}
}

func createRedpandaMockLogs() []s6service.LogEntry {
	return []s6service.LogEntry{
		{Content: "redpanda", Timestamp: time.Now()},
	}
}
