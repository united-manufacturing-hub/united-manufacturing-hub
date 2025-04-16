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

package agent_monitor_test

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// CustomMockS6Service extends s6.MockService to track the servicePath parameter passed to GetLogs
type CustomMockS6Service struct {
	*s6.MockService
	lastServicePath string
}

// GetLogs overrides the mock implementation to track the servicePath parameter
func (m *CustomMockS6Service) GetLogs(ctx context.Context, servicePath string, fs filesystem.Service) ([]s6.LogEntry, error) {
	m.lastServicePath = servicePath
	return m.MockService.GetLogs(ctx, servicePath, fs)
}

// NewCustomMockS6Service creates a new custom mock S6 service
func NewCustomMockS6Service() *CustomMockS6Service {
	return &CustomMockS6Service{
		MockService: s6.NewMockService(),
	}
}

var _ = Describe("Agent Monitor Service", func() {
	var (
		service      *agent_monitor.AgentMonitorService
		mockFS       *filesystem.MockFileSystem
		mockS6       *s6.MockService
		ctx          context.Context
		mockCfg      config.FullConfig
		mockSnapshot fsm.SystemSnapshot
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		mockS6 = s6.NewMockService()
		ctx = context.Background()

		// Create a mock config
		mockCfg = config.FullConfig{
			Agent: config.AgentConfig{
				Location: map[int]string{
					1: "Plant",
					2: "Area",
					3: "Line",
				},
				ReleaseChannel: "stable",
			},
		}

		// Create a SystemSnapshot with the config
		mockSnapshot = fsm.SystemSnapshot{
			CurrentConfig: mockCfg,
			SnapshotTime:  time.Now(),
			Tick:          1,
		}

		// Setup default mock behaviors
		mockS6.GetLogsResult = []s6.LogEntry{
			{
				Timestamp: time.Now().Add(-10 * time.Minute),
				Content:   "INFO: Test log entry 1",
			},
			{
				Timestamp: time.Now().Add(-5 * time.Minute),
				Content:   "WARN: Test log entry 2",
			},
		}
	})

	Describe("Constructor functions", func() {
		It("should create a new agent monitor service with default S6 service", func() {

			service = agent_monitor.NewAgentMonitorService(agent_monitor.WithFilesystemService(mockFS))

			Expect(service).NotTo(BeNil())
			Expect(service.GetFilesystemService()).To(Equal(mockFS))
		})

		It("should create a new agent monitor service with provided S6 service", func() {

			service = agent_monitor.NewAgentMonitorService(agent_monitor.WithFilesystemService(mockFS), agent_monitor.WithS6Service(mockS6))

			Expect(service).NotTo(BeNil())
			Expect(service.GetFilesystemService()).To(Equal(mockFS))
		})
	})

	Describe("GetStatus", func() {
		BeforeEach(func() {
			service = agent_monitor.NewAgentMonitorService(agent_monitor.WithFilesystemService(mockFS), agent_monitor.WithS6Service(mockS6))

		})

		Context("when everything works correctly", func() {
			It("should return the agent status with all expected fields", func() {
				status, err := service.Status(ctx, mockSnapshot)

				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())

				// Check location was copied from config
				Expect(status.Location).To(Equal(mockCfg.Agent.Location))

				// Check latency is initialized
				Expect(status.Latency).NotTo(BeNil())

				// Check logs were retrieved
				Expect(status.AgentLogs).To(HaveLen(2))
				Expect(status.AgentLogs[0].Content).To(Equal("INFO: Test log entry 1"))
				Expect(status.AgentLogs[1].Content).To(Equal("WARN: Test log entry 2"))

				// Check metrics are initialized
				Expect(status.AgentMetrics).NotTo(BeNil())

				// Check release info
				Expect(status.Release).NotTo(BeNil())
				Expect(status.Release.Channel).To(Equal("stable"))
				// Note: We can't check actual version values as they come from the version package
			})
		})

		Context("when config has empty location", func() {
			It("should return agent status with empty location map", func() {
				// Create config with nil location
				configWithNilLoc := mockCfg
				configWithNilLoc.Agent.Location = nil

				// Update snapshot with the modified config
				snapshotWithNilLoc := mockSnapshot
				snapshotWithNilLoc.CurrentConfig = configWithNilLoc

				status, err := service.Status(ctx, snapshotWithNilLoc)

				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())
				// Instead of expecting empty map, expect the default location
				Expect(status.Location).To(HaveKeyWithValue(0, "Unknown location"))
			})
		})

		Context("when S6 service encounters an error getting logs", func() {
			BeforeEach(func() {
				mockS6.GetLogsError = errors.New("failed to retrieve logs")
			})

			It("should return an error", func() {

				status, err := service.Status(ctx, mockSnapshot)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to retrieve logs"))
				Expect(status).To(BeNil())
			})
		})

		Context("when context is canceled", func() {
			It("should propagate context cancellation to S6 service", func() {
				canceledCtx, cancel := context.WithCancel(ctx)

				// Configure the S6 mock to return a context.Canceled error
				mockS6.GetLogsError = context.Canceled

				// Cancel the context
				cancel()

				// Call the function under test

				status, err := service.Status(canceledCtx, mockSnapshot)

				// Verify the error is correctly propagated
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get logs from S6 service"))
				Expect(status).To(BeNil())
			})
		})
	})

	Describe("Agent Logs", func() {
		BeforeEach(func() {

			service = agent_monitor.NewAgentMonitorService(agent_monitor.WithFilesystemService(mockFS), agent_monitor.WithS6Service(mockS6))

		})

		Context("when logs are available", func() {
			It("should return log entries from S6 service", func() {
				status, err := service.Status(ctx, mockSnapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.AgentLogs).To(HaveLen(2))
				Expect(status.AgentLogs[0].Content).To(ContainSubstring("INFO"))
				Expect(status.AgentLogs[1].Content).To(ContainSubstring("WARN"))
			})

			It("should use the correct service path", func() {
				_, err := service.Status(ctx, mockSnapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockS6.GetLogsCalled).To(BeTrue())
				// If we had access to mockS6's internal call parameters,
				// we would check that it was called with the expected service path
			})
		})

		Context("when S6 service returns an error", func() {
			BeforeEach(func() {
				mockS6.GetLogsError = errors.New("mock error retrieving logs")
			})

			It("should return the error", func() {
				status, err := service.Status(ctx, mockSnapshot)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get logs from S6 service"))
				Expect(status).To(BeNil())
			})
		})

		Context("when context is canceled", func() {
			It("should respect context cancellation and return error", func() {
				canceledCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				// Set up mock to return context.Canceled
				mockS6.GetLogsError = context.Canceled

				status, err := service.Status(canceledCtx, mockSnapshot)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get logs from S6 service"))
				Expect(err.Error()).To(ContainSubstring("context canceled"))
				Expect(status).To(BeNil())
			})
		})
	})

	Describe("getReleaseInfo", func() {
		BeforeEach(func() {

			service = agent_monitor.NewAgentMonitorService(agent_monitor.WithFilesystemService(mockFS), agent_monitor.WithS6Service(mockS6))

		})

		It("should return release info with channel from config", func() {
			// Since getReleaseInfo is an unexported method, test it implicitly through GetStatus

			status, err := service.Status(ctx, mockSnapshot)

			Expect(err).NotTo(HaveOccurred())
			Expect(status.Release).NotTo(BeNil())
			Expect(status.Release.Channel).To(Equal("stable"))

			// We could also test with a different release channel
			alternativeCfg := mockCfg
			alternativeCfg.Agent.ReleaseChannel = "testing"

			// Create a new snapshot with the alternative config
			alternativeSnapshot := mockSnapshot
			alternativeSnapshot.CurrentConfig = alternativeCfg

			status, err = service.Status(ctx, alternativeSnapshot)

			Expect(err).NotTo(HaveOccurred())
			Expect(status.Release.Channel).To(Equal("testing"))
		})

		It("should include version information", func() {

			status, err := service.Status(ctx, mockSnapshot)

			Expect(err).NotTo(HaveOccurred())

			// The actual version values come from the version package,
			// so we just check that they exist, not their specific values
			Expect(status.Release.Version).NotTo(BeEmpty())
			Expect(status.Release.Versions).NotTo(BeNil())
		})
	})

	Describe("Integration with filesystem and s6", func() {
		It("should correctly get logs from the expected service path", func() {
			// Create a custom mock S6 service to track the servicePath parameter
			customMockS6 := NewCustomMockS6Service()
			customMockS6.GetLogsResult = []s6.LogEntry{
				{
					Timestamp: time.Now(),
					Content:   "Test log",
				},
			}

			// Create a new service with our custom mock
			service = agent_monitor.NewAgentMonitorService(agent_monitor.WithFilesystemService(mockFS), agent_monitor.WithS6Service(customMockS6))

			// Call the method under test through Status
			_, err := service.Status(ctx, mockSnapshot)
			Expect(err).NotTo(HaveOccurred())

			// Verify the correct path was used
			expectedPath := filepath.Join(constants.S6BaseDir, "umh-core")
			Expect(customMockS6.lastServicePath).To(Equal(expectedPath))
		})
	})
})
