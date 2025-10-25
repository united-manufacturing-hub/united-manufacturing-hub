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

package agent_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/agent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	agent_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("AgentMonitorWorker", func() {
	var (
		worker      *agent.AgentMonitorWorker
		mockService *agent_monitor_service.MockService
		ctx         context.Context
		fs          filesystem.Service
	)

	BeforeEach(func() {
		ctx = context.Background()
		fs = filesystem.NewDefaultService()
		mockService = agent_monitor_service.NewMockService(fs)
		worker = agent.NewAgentMonitorWorker("test-id", "test-agent", mockService)
	})

	Describe("CollectObservedState", func() {
		Context("when agent is healthy", func() {
			BeforeEach(func() {
				mockService.SetupMockForHealthyState()
			})

			It("should return observed state with Active health", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(observed).NotTo(BeNil())

				agentObserved := observed.(*agent.AgentMonitorObservedState)
				Expect(agentObserved.ServiceInfo).NotTo(BeNil())
				Expect(agentObserved.ServiceInfo.OverallHealth).To(Equal(models.Active))
				Expect(agentObserved.ServiceInfo.LatencyHealth).To(Equal(models.Active))
				Expect(agentObserved.ServiceInfo.ReleaseHealth).To(Equal(models.Active))
			})

			It("should populate location information", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				agentObserved := observed.(*agent.AgentMonitorObservedState)
				Expect(agentObserved.ServiceInfo.Location).NotTo(BeEmpty())
			})

			It("should populate release information", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				agentObserved := observed.(*agent.AgentMonitorObservedState)
				Expect(agentObserved.ServiceInfo.Release).NotTo(BeNil())
				Expect(agentObserved.ServiceInfo.Release.Channel).NotTo(BeEmpty())
				Expect(agentObserved.ServiceInfo.Release.Version).NotTo(BeEmpty())
			})

			It("should set CollectedAt timestamp", func() {
				before := time.Now()
				observed, err := worker.CollectObservedState(ctx)
				after := time.Now()
				Expect(err).NotTo(HaveOccurred())

				agentObserved := observed.(*agent.AgentMonitorObservedState)
				Expect(agentObserved.CollectedAt).NotTo(BeZero())
				Expect(agentObserved.CollectedAt).To(BeTemporally(">=", before))
				Expect(agentObserved.CollectedAt).To(BeTemporally("<=", after))
			})
		})

		Context("when agent is degraded", func() {
			BeforeEach(func() {
				mockService.SetupMockForDegradedState()
			})

			It("should return observed state with Degraded health", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				agentObserved := observed.(*agent.AgentMonitorObservedState)
				Expect(agentObserved.ServiceInfo.OverallHealth).To(Equal(models.Degraded))
				Expect(agentObserved.ServiceInfo.LatencyHealth).To(Equal(models.Degraded))
			})
		})

		Context("when Status returns an error", func() {
			BeforeEach(func() {
				testError := errors.New("failed to collect agent metrics")
				mockService.SetupMockForError(testError)
			})

			It("should return the error", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to collect agent metrics"))
				Expect(observed).To(BeNil())
			})
		})

		Context("when Status returns nil ServiceInfo", func() {
			BeforeEach(func() {
				mockService.GetStatusResult = nil
			})

			It("should handle nil ServiceInfo gracefully", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(observed).NotTo(BeNil())

				agentObserved := observed.(*agent.AgentMonitorObservedState)
				Expect(agentObserved.ServiceInfo).To(BeNil())
				Expect(agentObserved.CollectedAt).NotTo(BeZero())
			})
		})
	})

	Describe("DeriveDesiredState", func() {
		Context("with nil spec", func() {
			It("should return default desired state", func() {
				desired, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(desired).NotTo(BeNil())

				agentDesired := desired.(*agent.AgentMonitorDesiredState)
				Expect(agentDesired.ShutdownRequested()).To(BeFalse())
			})
		})

		Context("with empty spec", func() {
			It("should return default desired state", func() {
				desired, err := worker.DeriveDesiredState(struct{}{})
				Expect(err).NotTo(HaveOccurred())
				Expect(desired).NotTo(BeNil())

				agentDesired := desired.(*agent.AgentMonitorDesiredState)
				Expect(agentDesired.ShutdownRequested()).To(BeFalse())
			})
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()
			Expect(initialState).To(BeAssignableToTypeOf(&agent.StoppedState{}))
		})

		It("should return state with correct name", func() {
			initialState := worker.GetInitialState()
			Expect(initialState.String()).To(Equal("Stopped"))
		})
	})

	Describe("Worker interface implementation", func() {
		It("should implement fsmv2.Worker interface", func() {
			var _ fsmv2.Worker = worker
		})
	})

	Describe("Incremental log collection", func() {
		Context("lastLogTimestamp initialization", func() {
			It("should initialize lastLogTimestamp to time.Now() on first observation", func() {
				mockService.SetupMockForHealthyState()
				beforeCall := time.Now()

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())

				afterCall := time.Now()

				agentObserved, ok := observed.(*agent.AgentMonitorObservedState)
				Expect(ok).To(BeTrue())
				Expect(agentObserved).NotTo(BeNil())

				lastTimestamp := worker.GetLastLogTimestamp()
				Expect(lastTimestamp).To(BeTemporally(">=", beforeCall))
				Expect(lastTimestamp).To(BeTemporally("<=", afterCall))
			})

			It("should not read entire log history on first call", func() {
				mockService.SetupMockForHealthyState()
				mockService.SetInitialLogCount(1000)

				observed, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())

				agentObserved, ok := observed.(*agent.AgentMonitorObservedState)
				Expect(ok).To(BeTrue())

				Expect(agentObserved.ServiceInfo.AgentLogs).To(BeEmpty())
			})
		})

		Context("incremental log collection on subsequent calls", func() {
			It("should only collect logs since last observation", func() {
				mockService.SetupMockForHealthyState()

				firstObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())

				firstAgentObserved, ok := firstObserved.(*agent.AgentMonitorObservedState)
				Expect(ok).To(BeTrue())
				Expect(firstAgentObserved.ServiceInfo.AgentLogs).To(BeEmpty())

				mockService.AddNewLogs(agent_monitor_service.CreateMockLogEntries(2, "new log"))

				secondObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())

				secondAgentObserved, ok := secondObserved.(*agent.AgentMonitorObservedState)
				Expect(ok).To(BeTrue())

				Expect(secondAgentObserved.ServiceInfo.AgentLogs).To(HaveLen(2))
				Expect(secondAgentObserved.ServiceInfo.AgentLogs[0].Content).To(ContainSubstring("new log"))
				Expect(secondAgentObserved.ServiceInfo.AgentLogs[1].Content).To(ContainSubstring("new log"))
			})

			It("should update lastLogTimestamp after each collection", func() {
				mockService.SetupMockForHealthyState()

				firstObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(firstObserved).NotTo(BeNil())

				firstTimestamp := worker.GetLastLogTimestamp()

				time.Sleep(10 * time.Millisecond)

				mockService.AddNewLogs(agent_monitor_service.CreateMockLogEntries(1, "new log"))

				secondObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(secondObserved).NotTo(BeNil())

				secondTimestamp := worker.GetLastLogTimestamp()

				Expect(secondTimestamp).To(BeTemporally(">", firstTimestamp))
			})

			It("should return empty logs when no new logs available", func() {
				mockService.SetupMockForHealthyState()

				firstObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(firstObserved).NotTo(BeNil())

				secondObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())

				secondAgentObserved, ok := secondObserved.(*agent.AgentMonitorObservedState)
				Expect(ok).To(BeTrue())

				Expect(secondAgentObserved.ServiceInfo.AgentLogs).To(BeEmpty())
			})

			It("should handle multiple incremental collections", func() {
				mockService.SetupMockForHealthyState()

				_, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())

				mockService.AddNewLogs(agent_monitor_service.CreateMockLogEntries(1, "log 1"))
				secondObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())
				secondAgentObserved := secondObserved.(*agent.AgentMonitorObservedState)
				Expect(secondAgentObserved.ServiceInfo.AgentLogs).To(HaveLen(1))

				mockService.AddNewLogs(agent_monitor_service.CreateMockLogEntries(2, "log 2"))
				thirdObserved, err := worker.CollectObservedState(ctx)
				Expect(err).ToNot(HaveOccurred())
				thirdAgentObserved := thirdObserved.(*agent.AgentMonitorObservedState)
				Expect(thirdAgentObserved.ServiceInfo.AgentLogs).To(HaveLen(2))
			})
		})

		Context("memory management", func() {
			It("should not accumulate logs in worker memory", func() {
				mockService.SetupMockForHealthyState()

				for range 100 {
					mockService.AddNewLogs(agent_monitor_service.CreateMockLogEntries(1, "log"))
					_, err := worker.CollectObservedState(ctx)
					Expect(err).ToNot(HaveOccurred())
				}

			})
		})
	})
})
