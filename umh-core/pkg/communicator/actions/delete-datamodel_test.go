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
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Helper function for delete action that doesn't need base64 encoding
func structToMap(v interface{}) map[string]interface{} {
	data, err := json.Marshal(v)
	Expect(err).ToNot(HaveOccurred())

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	Expect(err).ToNot(HaveOccurred())

	return result
}

var _ = Describe("DeleteDataModelAction", func() {
	var (
		action          *actions.DeleteDataModelAction
		mockConfigMgr   *config.MockConfigManager
		outboundChannel chan *models.UMHMessage
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
	)

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		mockConfigMgr = config.NewMockConfigManager()
		outboundChannel = make(chan *models.UMHMessage, 10)

		// Setup mock config with some existing data models
		mockConfigMgr.Config = config.FullConfig{
			DataModels: []config.DataModelsConfig{
				{
					Name: "test-model",
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Description: "Test data model",
							Structure: map[string]config.Field{
								"field1": {
									Type: "timeseries-string",
								},
							},
						},
					},
				},
				{
					Name: "another-model",
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Description: "Another test data model",
							Structure: map[string]config.Field{
								"field2": {
									Type: "timeseries-number",
								},
							},
						},
					},
				},
			},
		}

		action = actions.NewDeleteDataModelAction(
			userEmail,
			actionUUID,
			instanceUUID,
			outboundChannel,
			mockConfigMgr,
		)
	})

	AfterEach(func() {
		close(outboundChannel)
	})

	Describe("NewDeleteDataModelAction", func() {
		It("should create a new action with correct fields", func() {
			Expect(action).ToNot(BeNil())
		})
	})

	Describe("Parse", func() {
		Context("with valid payload", func() {
			It("should parse successfully", func() {
				payload := models.DeleteDataModelPayload{
					Name: "test-model",
				}

				err := action.Parse(structToMap(payload))

				Expect(err).ToNot(HaveOccurred())
				parsedPayload := action.GetParsedPayload()
				Expect(parsedPayload.Name).To(Equal("test-model"))
			})
		})

		Context("with invalid payload type", func() {
			It("should return error", func() {
				invalidPayload := "not a valid payload"

				err := action.Parse(invalidPayload)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
			})
		})

		Context("with nil payload", func() {
			It("should return error", func() {
				err := action.Parse(nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
			})
		})
	})

	Describe("Validate", func() {
		Context("with valid payload", func() {
			BeforeEach(func() {
				payload := models.DeleteDataModelPayload{
					Name: "test-model",
				}
				err := action.Parse(structToMap(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate successfully", func() {
				err := action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with missing name", func() {
			BeforeEach(func() {
				payload := models.DeleteDataModelPayload{
					Name: "",
				}
				err := action.Parse(structToMap(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return validation error", func() {
				err := action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("missing required field Name"))
			})
		})
	})

	Describe("Execute", func() {
		Context("with successful configuration update", func() {
			BeforeEach(func() {
				payload := models.DeleteDataModelPayload{
					Name: "test-model",
				}
				err := action.Parse(structToMap(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should execute successfully", func() {
				response, metadata, err := action.Execute()

				Expect(err).ToNot(HaveOccurred())
				Expect(metadata).To(BeNil())
				Expect(response).ToNot(BeNil())

				responseMap, ok := response.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(responseMap["name"]).To(Equal("test-model"))
				Expect(responseMap["deleted"]).To(Equal(true))

				// Verify config manager was called
				Expect(mockConfigMgr.AtomicDeleteDataModelCalled).To(BeTrue())
			})

			It("should send correct action replies to outbound channel", func() {
				go func() {
					defer GinkgoRecover()
					_, _, _ = action.Execute()
				}()

				// Should receive multiple messages
				var messages []*models.UMHMessage
				Eventually(func() int {
					select {
					case msg := <-outboundChannel:
						messages = append(messages, msg)
					case <-time.After(100 * time.Millisecond):
						// Timeout to prevent hanging
					}
					return len(messages)
				}, "1s").Should(BeNumerically(">=", 2))

				// Verify we received some messages
				Expect(len(messages)).To(BeNumerically(">", 0))
			})
		})

		Context("when data model does not exist", func() {
			BeforeEach(func() {
				payload := models.DeleteDataModelPayload{
					Name: "non-existent-model",
				}
				err := action.Parse(structToMap(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return error", func() {
				_, _, err := action.Execute()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Failed to delete data model"))
				Expect(mockConfigMgr.AtomicDeleteDataModelCalled).To(BeTrue())
			})
		})

		Context("when config manager fails", func() {
			BeforeEach(func() {
				payload := models.DeleteDataModelPayload{
					Name: "test-model",
				}
				err := action.Parse(structToMap(payload))
				Expect(err).ToNot(HaveOccurred())

				// Configure mock to return error
				mockConfigMgr.WithAtomicDeleteDataModelError(errors.New("mock delete error"))
			})

			It("should return error", func() {
				_, _, err := action.Execute()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Failed to delete data model"))
				Expect(mockConfigMgr.AtomicDeleteDataModelCalled).To(BeTrue())
			})
		})
	})

	Describe("GetParsedPayload", func() {
		It("should return empty payload before parsing", func() {
			payload := action.GetParsedPayload()
			Expect(payload.Name).To(BeEmpty())
		})

		It("should return parsed payload after parsing", func() {
			originalPayload := models.DeleteDataModelPayload{
				Name: "test-model",
			}
			err := action.Parse(structToMap(originalPayload))
			Expect(err).ToNot(HaveOccurred())

			payload := action.GetParsedPayload()
			Expect(payload.Name).To(Equal("test-model"))
		})
	})
})
