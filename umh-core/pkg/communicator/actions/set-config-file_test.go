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
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// memFileInfo already defined in get-config-file_test.go

var _ = Describe("SetConfigFile", func() {
	var (
		action           *actions.SetConfigFileAction
		userEmail        string
		actionUUID       uuid.UUID
		instanceUUID     uuid.UUID
		outboundChannel  chan *models.UMHMessage
		mockConfig       *config.MockConfigManager
		snapshotManager  *fsm.SnapshotManager
		configContent    string
		lastModifiedTime string
	)

	BeforeEach(func() {

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
		fixedTime := time.Date(2023, 5, 15, 10, 30, 0, 0, time.UTC)
		lastModifiedTime = fixedTime.Format(time.RFC3339)

		mockConfig = config.NewMockConfigManager()

		mockConfig.MockFileSystem.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
			if path == config.DefaultConfigPath {
				return []byte(configContent), nil
			}
			return nil, errors.New("file not found")
		})

		mockFileInfo := &memFileInfo{
			name:  "config.yaml",
			size:  100,
			mode:  0644,
			mtime: fixedTime,
			dir:   false,
		}

		mockConfig.MockFileSystem.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
			if path == config.DefaultConfigPath {
				return mockFileInfo, nil
			}
			return nil, errors.New("file not found")
		})

		mockConfig.MockFileSystem.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
			return nil
		})

		snapshotManager = fsm.NewSnapshotManager()
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{})

		action = actions.NewSetConfigFileAction(userEmail, actionUUID, instanceUUID, outboundChannel, snapshotManager, mockConfig)
	})

	AfterEach(func() {
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid payload", func() {
			payload := map[string]interface{}{
				"content":          configContent,
				"lastModifiedTime": lastModifiedTime,
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle invalid payload format", func() {
			payload := "invalid payload"
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation with valid payload", func() {
			payload := map[string]interface{}{
				"content":          configContent,
				"lastModifiedTime": lastModifiedTime,
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation with empty content", func() {
			payload := map[string]interface{}{
				"content":          "",
				"lastModifiedTime": lastModifiedTime,
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("content cannot be empty"))
		})

		It("should fail validation with empty lastModifiedTime", func() {
			payload := map[string]interface{}{
				"content":          configContent,
				"lastModifiedTime": "",
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("last modified time cannot be zero"))
		})
	})

	Describe("Execute", func() {
		BeforeEach(func() {
			payload := map[string]interface{}{
				"content":          configContent,
				"lastModifiedTime": lastModifiedTime,
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully update the config file", func() {
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			response, ok := result.(models.SetConfigFileResponse)
			Expect(ok).To(BeTrue(), "Result should be a SetConfigFileResponse")
			Expect(response.Content).To(Equal(configContent))
			Expect(response.LastModifiedTime).To(Equal(lastModifiedTime))
			Expect(response.Success).To(BeTrue())

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should detect concurrent modification", func() {
			differentTime := time.Date(2023, 5, 16, 10, 30, 0, 0, time.UTC)
			mockFileInfo := &memFileInfo{
				name:  "config.yaml",
				size:  100,
				mode:  0644,
				mtime: differentTime,
				dir:   false,
			}

			mockConfig.MockFileSystem.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == config.DefaultConfigPath {
					return mockFileInfo, nil
				}
				return nil, errors.New("file not found")
			})

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("concurrent modification detected"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should handle file stat errors", func() {
			mockConfig.MockFileSystem.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				return nil, errors.New("simulated stat error")
			})

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config file info"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should handle file write errors", func() {
			mockConfig.MockFileSystem.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return errors.New("simulated write error")
			})

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write config file"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should handle Stat error after successful write", func() {
			// first Stat call succeeds, second fails
			var statCallCount int
			mockConfig.MockFileSystem.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				statCallCount++
				if statCallCount == 1 {
					// First call to get current file info succeeds
					return &memFileInfo{
						name:  "config.yaml",
						size:  100,
						mode:  0644,
						mtime: time.Date(2023, 5, 15, 10, 30, 0, 0, time.UTC),
						dir:   false,
					}, nil
				}
				return nil, errors.New("simulated stat error after write")
			})

			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			response, ok := result.(models.SetConfigFileResponse)
			Expect(ok).To(BeTrue(), "Result should be a SetConfigFileResponse")
			Expect(response.Success).To(BeTrue())
			// LastModifiedTime should be empty if we failed to stat the file after writing
			Expect(response.LastModifiedTime).To(Equal(""))

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})

		It("should respect context timeout", func() {
			// setup filesystem to delay longer than the context timeout
			mockConfig.MockFileSystem.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				select {
				case <-time.After(2 * time.Second):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write config file"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// there should be a message sent to the outbound channel
			Eventually(outboundChannel).Should(Receive())
		})
	})
})
