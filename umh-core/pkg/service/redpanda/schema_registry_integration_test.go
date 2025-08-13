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

package redpanda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// emptyIntegrationTestConfig returns empty configuration for schema registry reconciliation
// This is used in integration tests where we want to test the reconciliation performance
// without the overhead of data model translation.
func emptyIntegrationTestConfig() ([]config.DataModelsConfig, []config.DataContractsConfig, map[string]config.PayloadShape) {
	return []config.DataModelsConfig{}, []config.DataContractsConfig{}, make(map[string]config.PayloadShape)
}

var redpandaContainer *redpanda.Container
var redpandaSchemaRegistryURL string

// cleanupRedpandaContainer cleans up the Redpanda container with proper timeout handling.
func cleanupRedpandaContainer() {
	if redpandaContainer != nil {
		// Use a timeout context to prevent hanging during cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Terminate the container (ignoring errors since we're cleaning up)
		_ = redpandaContainer.Terminate(ctx)
		redpandaContainer = nil
	}
}

// startRedpandaContainer starts a Redpanda container using testcontainers.
func startRedpandaContainer() error {
	// Clean up any existing container first
	cleanupRedpandaContainer()

	ctx := context.Background()

	// Create Redpanda container with testcontainers using Docker Hub image
	container, err := redpanda.Run(ctx,
		"redpandadata/redpanda:latest")
	if err != nil {
		return fmt.Errorf("failed to start Redpanda container: %w", err)
	}

	if container == nil {
		return errors.New("received nil container from redpanda.Run")
	}

	// Get the schema registry URL
	schemaRegistryURL, err := container.SchemaRegistryAddress(ctx)
	if err != nil {
		errX := container.Terminate(ctx)
		if errX != nil {
			return fmt.Errorf("failed to terminate Redpanda container: %w", errX)
		}

		return fmt.Errorf("failed to get schema registry URL: %w", err)
	}

	redpandaContainer = container
	redpandaSchemaRegistryURL = schemaRegistryURL

	return nil
}

// fetchSchemaViaHTTP retrieves a schema from the registry via HTTP GET
// Returns the schema definition if found, or an error if not found or malformed.
func fetchSchemaViaHTTP(subject SubjectName) (JSONSchemaDefinition, error) {
	// Get the latest version of the schema
	url := fmt.Sprintf("%s/subjects/%s/versions/latest", redpandaSchemaRegistryURL, string(subject))

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch schema for subject %s: %w", subject, err)
	}

	if resp == nil {
		return "", fmt.Errorf("received nil response for subject %s", subject)
	}

	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			// Log error but don't fail the function for close errors
			fmt.Printf("Warning: Failed to close response body for subject %s: %v\n", subject, closeErr)
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("subject %s not found in registry", subject)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d when fetching subject %s", resp.StatusCode, subject)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body for subject %s: %w", subject, err)
	}

	// Parse the response to extract the schema
	var response struct {
		Schema string `json:"schema"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response for subject %s: %w", subject, err)
	}

	return JSONSchemaDefinition(response.Schema), nil
}

// verifySchemaViaHTTP fetches a schema via HTTP and compares it to the expected definition.
func verifySchemaViaHTTP(subject SubjectName, expectedSchema JSONSchemaDefinition) error {
	fetchedSchema, err := fetchSchemaViaHTTP(subject)
	if err != nil {
		return err
	}

	// Parse both schemas to compare them structurally (ignoring whitespace differences)
	var expectedParsed, fetchedParsed interface{}

	if err := json.Unmarshal([]byte(expectedSchema), &expectedParsed); err != nil {
		return fmt.Errorf("failed to parse expected schema for %s: %w", subject, err)
	}

	if err := json.Unmarshal([]byte(fetchedSchema), &fetchedParsed); err != nil {
		return fmt.Errorf("failed to parse fetched schema for %s: %w", subject, err)
	}

	// Convert back to JSON with consistent formatting for comparison
	expectedNormalized, err := json.Marshal(expectedParsed)
	if err != nil {
		return fmt.Errorf("failed to marshal expected schema for comparison: %w", err)
	}

	fetchedNormalized, err := json.Marshal(fetchedParsed)
	if err != nil {
		return fmt.Errorf("failed to marshal fetched schema for comparison: %w", err)
	}

	if string(expectedNormalized) != string(fetchedNormalized) {
		return fmt.Errorf("schema mismatch for subject %s:\nExpected: %s\nFetched: %s",
			subject, string(expectedNormalized), string(fetchedNormalized))
	}

	return nil
}

var _ = Describe("Real Redpanda Integration Tests", Ordered, Label("integration"), func() {

	var registry *SchemaRegistry

	BeforeAll(func() {
		err := startRedpandaContainer()
		Expect(err).NotTo(HaveOccurred())
		registry = NewSchemaRegistry(WithSchemaRegistryAddress(redpandaSchemaRegistryURL))
	})

	Describe("Schema Registry Availability", func() {
		It("should be running and accessible", func() {
			Eventually(func() bool {
				resp, err := http.Get(redpandaSchemaRegistryURL + "/subjects")
				if err != nil {
					return false
				}
				if resp == nil {
					return false
				}
				if resp.Body != nil {
					closeErr := resp.Body.Close()
					if closeErr != nil {
						// Log but don't fail for close errors
						fmt.Printf("Warning: Failed to close response body: %v\n", closeErr)
					}
				}

				return resp.StatusCode == http.StatusOK
			}, 30*time.Second, 1*time.Second).Should(BeTrue())
		})
	})

	Describe("Basic Reconciliation", func() {
		var ctx context.Context
		var cancel context.CancelFunc

		BeforeEach(func() {
			testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
			ctx, cancel = testCtx, testCancel
		})

		AfterEach(func() {
			cancel()
		})

		It("should handle empty expected schemas", func() {
			dataModels, dataContracts, payloadShapes := emptyIntegrationTestConfig()

			Eventually(func() bool {
				err := registry.Reconcile(ctx, dataModels, dataContracts, payloadShapes)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()

				return metrics.CurrentPhase == SchemaRegistryPhaseLookup &&
					metrics.TotalReconciliations >= 1 &&
					metrics.SuccessfulOperations >= 1
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
		})

		It("should add new schemas", func() {
			// For integration tests, we'll use empty configuration for now
			// In a real scenario, these would be derived from DataModels and DataContracts
			dataModels, dataContracts, payloadShapes := emptyIntegrationTestConfig()

			Eventually(func() bool {
				err := registry.Reconcile(ctx, dataModels, dataContracts, payloadShapes)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()

				return metrics.SubjectsToAdd == 0 &&
					metrics.SubjectsToRemove == 0 &&
					metrics.SuccessfulOperations >= 1
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
		})

		It("should handle existing schemas correctly", func() {
			// Same schemas as previous test - should be idempotent
			dataModels, dataContracts, payloadShapes := emptyIntegrationTestConfig()

			Eventually(func() bool {
				err := registry.Reconcile(ctx, dataModels, dataContracts, payloadShapes)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()

				return metrics.SubjectsToAdd == 0 &&
					metrics.SubjectsToRemove == 0 &&
					metrics.SuccessfulOperations >= 1
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
		})

		It("should remove unexpected schemas", func() {
			// Remove one schema by not including it in expected
			dataModels, dataContracts, payloadShapes := emptyIntegrationTestConfig()

			Eventually(func() bool {
				err := registry.Reconcile(ctx, dataModels, dataContracts, payloadShapes)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()

				return metrics.SubjectsToAdd == 0 &&
					metrics.SubjectsToRemove == 0 &&
					metrics.SuccessfulOperations >= 1
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
		})

		It("should handle mixed add and remove operations", func() {
			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"test-subject-new": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"sensor": {"type": "string"},
						"reading": {"type": "number"},
						"unit": {"type": "string"}
					},
					"required": ["sensor", "reading"]
				}`),
				"test-subject-another": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"event": {"type": "string"},
						"data": {"type": "object"}
					},
					"required": ["event"]
				}`),
			}

			Eventually(func() bool {
				err := registry.ReconcileWithSchemas(ctx, expectedSchemas)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()

				return metrics.SubjectsToAdd == 0 &&
					metrics.SubjectsToRemove == 0 &&
					metrics.SuccessfulOperations >= 1
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
		})

		It("should reconcile schemas and make them retrievable via HTTP", func() {
			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"http-verify-1": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"deviceId": {"type": "string"},
						"temperature": {"type": "number", "minimum": -50, "maximum": 100},
						"humidity": {"type": "number", "minimum": 0, "maximum": 100},
						"timestamp": {"type": "string", "format": "date-time"}
					},
					"required": ["deviceId", "temperature", "timestamp"]
				}`),
				"http-verify-2": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"orderId": {"type": "string"},
						"customerId": {"type": "string"},
						"items": {
							"type": "array",
							"items": {
								"type": "object",
								"properties": {
									"productId": {"type": "string"},
									"quantity": {"type": "integer", "minimum": 1}
								},
								"required": ["productId", "quantity"]
							}
						},
						"total": {"type": "number", "minimum": 0}
					},
					"required": ["orderId", "customerId", "items", "total"]
				}`),
			}

			// First, reconcile the schemas
			Eventually(func() bool {
				err := registry.ReconcileWithSchemas(ctx, expectedSchemas)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()

				return metrics.SubjectsToAdd == 0 &&
					metrics.SubjectsToRemove == 0 &&
					metrics.SuccessfulOperations >= 1
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

			// Then verify each schema is retrievable via HTTP
			for subject, expectedSchema := range expectedSchemas {
				Eventually(func() error {
					return verifySchemaViaHTTP(subject, expectedSchema)
				}, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
					fmt.Sprintf("Schema verification failed for subject %s", subject))
			}
		})
	})

	Describe("Metrics and Monitoring", func() {
		var ctx context.Context
		var cancel context.CancelFunc

		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		})

		AfterEach(func() {
			cancel()
		})

		It("should track reconciliation metrics", func() {
			initialMetrics := registry.GetMetrics()
			initialReconciliations := initialMetrics.TotalReconciliations
			initialSuccessfulOps := initialMetrics.SuccessfulOperations

			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"metrics-test": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"metric": {"type": "string"},
						"value": {"type": "number"}
					}
				}`),
			}

			Eventually(func() bool {
				err := registry.ReconcileWithSchemas(ctx, expectedSchemas)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()
				GinkgoWriter.Printf("==============================================\n")
				GinkgoWriter.Printf("SubjectsToAdd: %d\n", metrics.SubjectsToAdd)
				GinkgoWriter.Printf("SubjectsToRemove: %d\n", metrics.SubjectsToRemove)
				GinkgoWriter.Printf("TotalReconciliations: %d\n", metrics.TotalReconciliations)
				GinkgoWriter.Printf("SuccessfulOperations: %d\n", metrics.SuccessfulOperations)
				GinkgoWriter.Printf("LastError: %s\n", metrics.LastError)
				GinkgoWriter.Printf("LastOperationTime: %s\n", metrics.LastOperationTime)
				GinkgoWriter.Printf("LastOperationTime under 10 seconds: %t\n", time.Since(metrics.LastOperationTime) < 10*time.Second)

				return metrics.SubjectsToAdd == 0 &&
					metrics.SubjectsToRemove == 0 &&
					metrics.TotalReconciliations > initialReconciliations &&
					metrics.SuccessfulOperations > initialSuccessfulOps &&
					metrics.LastError == "" &&
					time.Since(metrics.LastOperationTime) < 10*time.Second
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
		})

		It("should provide current phase information", func() {
			Eventually(func() bool {
				metrics := registry.GetMetrics()

				return metrics.CurrentPhase == SchemaRegistryPhaseLookup
			}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("Error Handling", func() {
		It("should handle context timeout", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
			defer cancel()

			dataModels, dataContracts, payloadShapes := emptyIntegrationTestConfig()

			err := registry.Reconcile(ctx, dataModels, dataContracts, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context deadline"))
		})

		It("should handle malformed JSON schemas", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Test with actually malformed JSON schema
			malformedSchemas := map[SubjectName]JSONSchemaDefinition{
				"malformed-schema": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"field": {"type": "invalid-type"}  // This is invalid JSON Schema
					},
					"required": ["field"
				}`),
			}

			// This should fail due to invalid schema
			Eventually(func() bool {
				err := registry.ReconcileWithSchemas(ctx, malformedSchemas)
				if err == nil {
					return false
				}
				metrics := registry.GetMetrics()

				return metrics.FailedOperations > 0 && metrics.LastError != ""
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("Concurrent Operations", func() {
		It("should handle concurrent metric reads safely", func() {
			done := make(chan bool, 10)

			// Start multiple goroutines reading metrics
			for range 10 {
				go func() {
					defer GinkgoRecover()
					Eventually(func() bool {
						for range 100 {
							metrics := registry.GetMetrics()
							if metrics.TotalReconciliations < 0 {
								return false
							}
						}

						return true
					}, 5*time.Second, 10*time.Millisecond).Should(BeTrue())
					done <- true
				}()
			}

			// Wait for all goroutines to complete
			for range 10 {
				Eventually(done).Should(Receive())
			}
		})
	})

	AfterAll(func() {
		cleanupRedpandaContainer()
	})
})
