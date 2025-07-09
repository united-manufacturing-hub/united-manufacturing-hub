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
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
)

var redpandaContainer *redpanda.Container
var redpandaSchemaRegistryURL string

// cleanupRedpandaContainer cleans up the Redpanda container with proper timeout handling
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

// startRedpandaContainer starts a Redpanda container using testcontainers
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

var _ = Describe("Real Redpanda Integration Tests", Ordered, func() {

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
				if resp.Body != nil {
					resp.Body.Close()
				}
				return resp.StatusCode == 200
			}, 30*time.Second, 1*time.Second).Should(BeTrue())
		})
	})

	Describe("Basic Reconciliation", func() {
		var ctx context.Context
		var cancel context.CancelFunc

		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		})

		AfterEach(func() {
			cancel()
		})

		It("should handle empty expected schemas", func() {
			expectedSchemas := make(map[SubjectName]JSONSchemaDefinition)

			Eventually(func() bool {
				err := registry.Reconcile(ctx, expectedSchemas)
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
			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"test-subject-1": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"timestamp": {"type": "string", "format": "date-time"},
						"value": {"type": "number"}
					},
					"required": ["id", "timestamp", "value"]
				}`),
				"test-subject-2": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"machineId": {"type": "string"},
						"state": {"type": "string", "enum": ["running", "stopped", "maintenance"]}
					},
					"required": ["machineId", "state"]
				}`),
			}

			Eventually(func() bool {
				err := registry.Reconcile(ctx, expectedSchemas)
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
			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"test-subject-1": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"timestamp": {"type": "string", "format": "date-time"},
						"value": {"type": "number"}
					},
					"required": ["id", "timestamp", "value"]
				}`),
				"test-subject-2": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"machineId": {"type": "string"},
						"state": {"type": "string", "enum": ["running", "stopped", "maintenance"]}
					},
					"required": ["machineId", "state"]
				}`),
			}

			Eventually(func() bool {
				err := registry.Reconcile(ctx, expectedSchemas)
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
			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"test-subject-1": JSONSchemaDefinition(`{
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"timestamp": {"type": "string", "format": "date-time"},
						"value": {"type": "number"}
					},
					"required": ["id", "timestamp", "value"]
				}`),
			}

			Eventually(func() bool {
				err := registry.Reconcile(ctx, expectedSchemas)
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
				err := registry.Reconcile(ctx, expectedSchemas)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()
				return metrics.SubjectsToAdd == 0 &&
					metrics.SubjectsToRemove == 0 &&
					metrics.SuccessfulOperations >= 1
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
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
				err := registry.Reconcile(ctx, expectedSchemas)
				if err != nil {
					return false
				}
				metrics := registry.GetMetrics()
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

			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"timeout-test": JSONSchemaDefinition(`{"type": "object"}`),
			}

			err := registry.Reconcile(ctx, expectedSchemas)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context deadline"))
		})

		It("should handle malformed JSON schemas", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			expectedSchemas := map[SubjectName]JSONSchemaDefinition{
				"malformed-test": JSONSchemaDefinition(`{invalid json}`),
			}

			// This should eventually fail consistently
			Consistently(func() bool {
				err := registry.Reconcile(ctx, expectedSchemas)
				if err == nil {
					return false
				}
				metrics := registry.GetMetrics()
				return metrics.FailedOperations > 0 && metrics.LastError != ""
			}, 3*time.Second, 500*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("Concurrent Operations", func() {
		It("should handle concurrent metric reads safely", func() {
			done := make(chan bool, 10)

			// Start multiple goroutines reading metrics
			for i := 0; i < 10; i++ {
				go func() {
					defer GinkgoRecover()
					Eventually(func() bool {
						for j := 0; j < 100; j++ {
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
			for i := 0; i < 10; i++ {
				Eventually(done).Should(Receive())
			}
		})
	})

	AfterAll(func() {
		cleanupRedpandaContainer()
	})
})
