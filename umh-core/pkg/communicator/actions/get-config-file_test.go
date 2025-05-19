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

package actions_test

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("GetConfigFile", func() {
	var (
		action          *actions.GetConfigFileAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		snapshotManager *fsm.SnapshotManager
		configContent   string
	)

	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		configContent = `{
			"agent": {
				"metricsPort": 8080,
				"location": {
					"0": "Enterprise",
					"1": "Site",
					"2": "Area"
				}
			},
			"dataFlow": []
		}`

		// Create mock config manager
		mockConfig = config.NewMockConfigManager()
		mockConfig.WithConfigAsString(configContent)

		// Setup mock filesystem with a config file
		mockConfig.MockFileSystem.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
			if path == config.DefaultConfigPath {
				return []byte(configContent), nil
			}
			return nil, errors.New("file not found")
		})

		// Initialize snapshot manager for system state
		snapshotManager = fsm.NewSnapshotManager()
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{})

		// Create the action
		action = actions.NewGetConfigFileAction(userEmail, actionUUID, instanceUUID, outboundChannel, snapshotManager, mockConfig)
	})

	AfterEach(func() {
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should not have any payload to parse", func() {
			payload := map[string]interface{}{}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		It("should pass validation", func() {
			err := action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Execute", func() {
		It("should return the config file content", func() {
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			response, ok := result.(models.GetConfigFileResponse)
			Expect(ok).To(BeTrue(), "Result should be a GetConfigFileResponse")
			Expect(response.Content).To(Equal(configContent))

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should include the last modified time in the response", func() {
			// Create a mock file info with specific modification time
			fixedTime := time.Date(2023, 5, 15, 10, 30, 0, 0, time.UTC)

			mockConfig.WithCacheModTime(fixedTime)

			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			response, ok := result.(models.GetConfigFileResponse)
			Expect(ok).To(BeTrue(), "Result should be a GetConfigFileResponse")
			Expect(response.Content).To(Equal(configContent))

			// Check that LastModifiedTime is set correctly
			expectedTimeStr := fixedTime.Format(time.RFC3339)
			Expect(response.LastModifiedTime).To(Equal(expectedTimeStr))

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should handle filesystem errors", func() {
			mockConfig.MockFileSystem.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				return nil, errors.New("simulated filesystem error")
			})
			mockConfig.WithGetConfigAsStringError(errors.New("simulated filesystem error"))

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should respect context timeout", func() {
			// Setup filesystem to delay longer than the context timeout
			mockConfig.MockFileSystem.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				select {
				case <-time.After(2 * time.Second):
					return []byte(configContent), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})
			mockConfig.WithConfigDelay(2 * time.Second)

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})
	})
})
