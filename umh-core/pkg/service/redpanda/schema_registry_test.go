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
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// SampleSchemas provides sample schemas for testing purposes
func SampleSchemas() map[SubjectName]JSONSchemaDefinition {
	return map[SubjectName]JSONSchemaDefinition{
		SubjectName("sensor-value"): JSONSchemaDefinition(`{
			"type": "object",
			"properties": {
				"timestamp": {"type": "string", "format": "date-time"},
				"value": {"type": "number"},
				"unit": {"type": "string"}
			},
			"required": ["timestamp", "value"]
		}`),
		SubjectName("machine-state"): JSONSchemaDefinition(`{
			"type": "object",
			"properties": {
				"machineId": {"type": "string"},
				"state": {"type": "string", "enum": ["running", "stopped", "maintenance"]},
				"timestamp": {"type": "string", "format": "date-time"}
			},
			"required": ["machineId", "state", "timestamp"]
		}`),
		SubjectName("production-count"): JSONSchemaDefinition(`{
			"type": "object",
			"properties": {
				"count": {"type": "integer", "minimum": 0},
				"timestamp": {"type": "string", "format": "date-time"},
				"productId": {"type": "string"}
			},
			"required": ["count", "timestamp"]
		}`),
	}
}

var _ = Describe("SchemaRegistry", func() {
	var (
		mockRegistry *MockSchemaRegistry
		registry     *SchemaRegistry
		ctx          context.Context
		cancel       context.CancelFunc
	)

	BeforeEach(func() {
		mockRegistry = NewMockSchemaRegistry()
		mockRegistry.SetupTestSchemas()

		registry = NewSchemaRegistry()

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		if mockRegistry != nil {
			mockRegistry.Close()
		}
	})

	Describe("NewSchemaRegistry", func() {
		It("should create a new schema registry with correct initial state", func() {
			sr := NewSchemaRegistry()

			Expect(sr).NotTo(BeNil())
			Expect(sr.currentPhase).To(Equal(SchemaRegistryPhaseLookup))
			Expect(sr.httpClient).NotTo(BeNil())
		})
	})

	Describe("Reconcile", func() {
		Context("when in lookup phase", func() {
			BeforeEach(func() {
				registry.currentPhase = SchemaRegistryPhaseLookup
				// Override the address for testing by modifying the registry's lookup behavior
				overrideSchemaRegistryAddress(registry, mockRegistry.URL())
			})

			It("should successfully retrieve subjects and process through reconciliation", func() {
				err := registry.Reconcile(ctx, SampleSchemas())

				Expect(err).NotTo(HaveOccurred())
				// After reconciliation, we expect to have processed unknown schemas for removal
				// Since mock registry has schemas that aren't in SampleSchemas(), we should be in remove_unknown phase
				// or have completed the cycle and be back in lookup phase
				Expect(registry.currentPhase).To(Or(
					Equal(SchemaRegistryPhaseRemoveUnknown),
					Equal(SchemaRegistryPhaseAddNew),
					Equal(SchemaRegistryPhaseLookup),
				))

				// Check that subjects were retrieved during the lookup phase
				Expect(registry.registrySubjects).To(ContainElements(
					SubjectName("_sensor_data_v1_timeseries-number"),
					SubjectName("_sensor_data_v2_timeseries-number"),
					SubjectName("_pump_data_v1_timeseries-number"),
					SubjectName("_pump_data_v1_timeseries-string"),
					SubjectName("_motor_controller_v3_timeseries-number"),
					SubjectName("_motor_controller_v3_timeseries-string"),
					SubjectName("_string_data_v1_timeseries-string"),
				))
			})

			It("should handle empty subject list and proceed to add missing schemas", func() {
				// Create a new empty mock registry
				emptyMockRegistry := NewMockSchemaRegistry()
				defer emptyMockRegistry.Close()

				overrideSchemaRegistryAddress(registry, emptyMockRegistry.URL())

				err := registry.Reconcile(ctx, SampleSchemas())

				Expect(err).NotTo(HaveOccurred())
				// With empty registry and SampleSchemas() to add, we should be in add_new phase
				// or have completed adding and be back in lookup phase
				Expect(registry.currentPhase).To(Or(
					Equal(SchemaRegistryPhaseAddNew),
					Equal(SchemaRegistryPhaseLookup),
				))
				Expect(registry.registrySubjects).To(HaveLen(0))
			})

			It("should fail when context has insufficient time", func() {
				// Create a context with less than MinimumLookupTime
				shortCtx, shortCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				defer shortCancel()

				err := registry.Reconcile(shortCtx, SampleSchemas())

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insufficient time remaining in context"))
				Expect(registry.currentPhase).To(Equal(SchemaRegistryPhaseLookup)) // Should not transition
			})

			It("should handle network errors gracefully", func() {
				mockRegistry.SimulateNetworkError(true)

				err := registry.Reconcile(ctx, SampleSchemas())

				Expect(err).To(HaveOccurred())
				Expect(registry.currentPhase).To(Equal(SchemaRegistryPhaseLookup)) // Should not transition
			})

			It("should handle context cancellation", func() {
				// Cancel the context before making the request
				cancel()

				err := registry.Reconcile(ctx, SampleSchemas())

				Expect(err).To(HaveOccurred())
				Expect(registry.currentPhase).To(Equal(SchemaRegistryPhaseLookup)) // Should not transition
			})
		})

		Context("when in compare phase", func() {
			BeforeEach(func() {
				registry.currentPhase = SchemaRegistryPhaseCompare
				// Add mock URL override for this test as well
				overrideSchemaRegistryAddress(registry, mockRegistry.URL())
			})

			It("should complete comparison and proceed with reconciliation", func() {
				err := registry.Reconcile(ctx, SampleSchemas())

				Expect(err).NotTo(HaveOccurred())
				// Starting from compare phase, reconciliation should proceed to action phases
				// The exact phase depends on what work needs to be done
				Expect(registry.currentPhase).To(Or(
					Equal(SchemaRegistryPhaseRemoveUnknown),
					Equal(SchemaRegistryPhaseAddNew),
					Equal(SchemaRegistryPhaseLookup),
				))
			})
		})

		Context("when in add new phase", func() {
			BeforeEach(func() {
				registry.currentPhase = SchemaRegistryPhaseAddNew
				// Add mock URL override for this test
				overrideSchemaRegistryAddress(registry, mockRegistry.URL())
			})

			It("should complete add operation and return success", func() {
				err := registry.Reconcile(ctx, SampleSchemas())

				Expect(err).NotTo(HaveOccurred())
				// After add phase, we should either stay in add_new (more to add) or move to lookup (cycle complete)
				Expect(registry.currentPhase).To(Or(
					Equal(SchemaRegistryPhaseAddNew),
					Equal(SchemaRegistryPhaseLookup),
				))
			})
		})

		Context("when in unknown phase", func() {
			BeforeEach(func() {
				registry.currentPhase = SchemaRegistryPhase("unknown")
			})

			It("should return an error for unknown phase", func() {
				err := registry.Reconcile(ctx, SampleSchemas())

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown phase: unknown"))
			})
		})
	})

	Describe("lookup method", func() {
		BeforeEach(func() {
			overrideSchemaRegistryAddress(registry, mockRegistry.URL())
		})

		It("should check context deadline before making request", func() {
			// Create context with slightly more than MinimumLookupTime - should work
			exactCtx, exactCancel := context.WithTimeout(context.Background(), MinimumLookupTime+5*time.Millisecond)
			defer exactCancel()

			err, changePhase := registry.lookup(exactCtx)

			Expect(err).NotTo(HaveOccurred())
			Expect(changePhase).To(BeTrue()) // lookup should advance to next phase
		})

		It("should fail when context deadline is too close", func() {
			// Create context with less than MinimumLookupTime
			tooShortCtx, tooShortCancel := context.WithTimeout(context.Background(), MinimumLookupTime-1*time.Millisecond)
			defer tooShortCancel()

			err, changePhase := registry.lookup(tooShortCtx)

			Expect(err).To(HaveOccurred())
			Expect(changePhase).To(BeFalse()) // On error, should not change phase
			Expect(err.Error()).To(ContainSubstring("insufficient time remaining"))
		})

		It("should work with context without deadline", func() {
			ctxNoDeadline := context.Background()

			err, changePhase := registry.lookup(ctxNoDeadline)

			Expect(err).NotTo(HaveOccurred())
			Expect(changePhase).To(BeTrue()) // should advance to next phase
		})

		It("should handle HTTP errors properly", func() {
			mockRegistry.SimulateNetworkError(true)

			err, changePhase := registry.lookup(ctx)

			Expect(err).To(HaveOccurred())
			Expect(changePhase).To(BeFalse()) // On error, should not change phase
			Expect(err.Error()).To(ContainSubstring("schema registry lookup failed with status 500"))
		})
	})
})

// Helper function to override the schema registry address for testing
func overrideSchemaRegistryAddress(registry *SchemaRegistry, mockURL string) {
	// We need to modify the behavior of the lookup method to use our mock URL
	// Since we can't easily override the constant, we'll modify the http client
	// to use a custom transport that rewrites the URL

	originalTransport := registry.httpClient.Transport
	if originalTransport == nil {
		originalTransport = &MockTransport{mockURL: mockURL}
	} else {
		originalTransport = &MockTransport{
			mockURL:   mockURL,
			transport: originalTransport,
		}
	}
	registry.httpClient.Transport = originalTransport
}

// MockTransport wraps the default transport and redirects requests to our mock server
type MockTransport struct {
	mockURL   string
	transport http.RoundTripper
}

func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the host and scheme with our mock server
	if strings.Contains(req.URL.Host, "localhost:8081") {
		mockURL, err := url.Parse(m.mockURL)
		if err != nil || mockURL == nil {
			return nil, fmt.Errorf("failed to parse mock URL: %v", err)
		}
		req.URL.Scheme = mockURL.Scheme
		req.URL.Host = mockURL.Host
	}

	if m.transport != nil {
		return m.transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
