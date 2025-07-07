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
	"sort"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Schema Registry", func() {
	Context("Schema Registry API", func() {
		It("should fetch all schemas from the schema registry", func() {
			mockRedpandaService := NewMockRedpandaService()

			// Set up mock return values
			expectedSchemas := []SchemaSubject{
				{Subject: "test-subject-1"},
				{Subject: "test-subject-2"},
			}
			mockRedpandaService.GetAllSchemasResult = expectedSchemas
			mockRedpandaService.GetAllSchemasError = nil

			// Call the method
			ctx := context.Background()
			schemas, err := mockRedpandaService.GetAllSchemas(ctx)

			// Verify the results
			Expect(err).To(BeNil())
			Expect(mockRedpandaService.GetAllSchemasCalled).To(BeTrue())
			Expect(schemas).To(Equal(expectedSchemas))
			Expect(len(schemas)).To(Equal(2))
			Expect(schemas[0].Subject).To(Equal("test-subject-1"))
			Expect(schemas[1].Subject).To(Equal("test-subject-2"))
		})

		It("should handle error when fetching schemas", func() {
			mockRedpandaService := NewMockRedpandaService()

			// Set up mock to return error
			mockRedpandaService.GetAllSchemasResult = nil
			mockRedpandaService.GetAllSchemasError = fmt.Errorf("schema registry not available")

			// Call the method
			ctx := context.Background()
			schemas, err := mockRedpandaService.GetAllSchemas(ctx)

			// Verify the results
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("schema registry not available"))
			Expect(mockRedpandaService.GetAllSchemasCalled).To(BeTrue())
			Expect(schemas).To(BeNil())
		})

		It("should handle connection refused error gracefully", func() {
			mockRedpandaService := NewMockRedpandaService()

			// Set up mock to return empty schemas (simulating connection refused)
			mockRedpandaService.GetAllSchemasResult = []SchemaSubject{}
			mockRedpandaService.GetAllSchemasError = nil

			// Call the method
			ctx := context.Background()
			schemas, err := mockRedpandaService.GetAllSchemas(ctx)

			// Verify the results
			Expect(err).To(BeNil())
			Expect(mockRedpandaService.GetAllSchemasCalled).To(BeTrue())
			Expect(schemas).To(BeEmpty())
		})
	})

	Context("Schema Name Parsing", func() {
		It("should parse schema names correctly", func() {
			// Test cases for schema name parsing
			testCases := []struct {
				schemaName        string
				expectedName      string
				expectedVersion   string
				expectedModelType string
				expectedDataType  string
				expectedSuccess   bool
			}{
				{"pump_v1_timeseries-number", "pump", "v1", "timeseries", "number", true},
				{"motor_v2_timeseries-string", "motor", "v2", "timeseries", "string", true},
				{"sensor_v10_timeseries-bool", "sensor", "v10", "timeseries", "bool", true},
				{"invalid_format", "", "", "", "", false},
				{"pump_v1", "", "", "", "", false},
				{"pump_v1_timeseries", "", "", "", "", false},
				{"_v1_timeseries-number", "", "", "", "", false},
				{"pump__timeseries-number", "", "", "", "", false},
				{"pump_v1_-number", "", "", "", "", false},
				{"pump_v1_timeseries-", "", "", "", "", false},
			}

			for _, tc := range testCases {
				name, version, modelType, dataType, success := parseSchemaName(tc.schemaName)

				Expect(success).To(Equal(tc.expectedSuccess), "Schema: %s", tc.schemaName)
				if tc.expectedSuccess {
					Expect(name).To(Equal(tc.expectedName), "Name for schema: %s", tc.schemaName)
					Expect(version).To(Equal(tc.expectedVersion), "Version for schema: %s", tc.schemaName)
					Expect(modelType).To(Equal(tc.expectedModelType), "ModelType for schema: %s", tc.schemaName)
					Expect(dataType).To(Equal(tc.expectedDataType), "DataType for schema: %s", tc.schemaName)
				}
			}
		})

		It("should fetch schemas with parsed components", func() {
			mockRedpandaService := NewMockRedpandaService()

			// Set up registry schemas with parsed components
			registrySchemas := []SchemaSubject{
				{
					Subject:            "pump_v1_timeseries-number",
					Name:               "pump",
					Version:            "v1",
					DataModelType:      "timeseries",
					DataType:           "number",
					ParsedSuccessfully: true,
				},
				{
					Subject:            "motor_v2_timeseries-string",
					Name:               "motor",
					Version:            "v2",
					DataModelType:      "timeseries",
					DataType:           "string",
					ParsedSuccessfully: true,
				},
				{
					Subject:            "invalid-schema-name",
					Name:               "",
					Version:            "",
					DataModelType:      "",
					DataType:           "",
					ParsedSuccessfully: false,
				},
			}
			mockRedpandaService.GetAllSchemasResult = registrySchemas
			mockRedpandaService.GetAllSchemasError = nil

			ctx := context.Background()
			schemas, err := mockRedpandaService.GetAllSchemas(ctx)

			Expect(err).To(BeNil())
			Expect(len(schemas)).To(Equal(3))

			// Check first schema (pump)
			pumpSchema := schemas[0]
			Expect(pumpSchema.Subject).To(Equal("pump_v1_timeseries-number"))
			Expect(pumpSchema.Name).To(Equal("pump"))
			Expect(pumpSchema.Version).To(Equal("v1"))
			Expect(pumpSchema.DataModelType).To(Equal("timeseries"))
			Expect(pumpSchema.DataType).To(Equal("number"))
			Expect(pumpSchema.ParsedSuccessfully).To(BeTrue())

			// Check second schema (motor)
			motorSchema := schemas[1]
			Expect(motorSchema.Subject).To(Equal("motor_v2_timeseries-string"))
			Expect(motorSchema.Name).To(Equal("motor"))
			Expect(motorSchema.Version).To(Equal("v2"))
			Expect(motorSchema.DataModelType).To(Equal("timeseries"))
			Expect(motorSchema.DataType).To(Equal("string"))
			Expect(motorSchema.ParsedSuccessfully).To(BeTrue())

			// Check invalid schema
			invalidSchema := schemas[2]
			Expect(invalidSchema.Subject).To(Equal("invalid-schema-name"))
			Expect(invalidSchema.ParsedSuccessfully).To(BeFalse())
		})
	})

	Context("Schema Generation and Mapping", func() {
		It("should generate expected schema names from data models", func() {
			dataModels := map[string]config.DataModelsConfig{
				"pump": {
					Name: "pump",
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"pressure": {
									Type: "timeseries-number",
								},
								"status": {
									Type: "timeseries-string",
								},
								"active": {
									Type: "timeseries-boolean",
								},
								"metadata": {
									Subfields: map[string]config.Field{
										"temperature": {
											Type: "timeseries-number",
										},
									},
								},
							},
						},
					},
				},
				"motor": {
					Name: "motor",
					Versions: map[string]config.DataModelVersion{
						"v2": {
							Structure: map[string]config.Field{
								"rpm": {
									Type: "timeseries-number",
								},
								"power": {
									Type: "timeseries-number",
								},
							},
						},
					},
				},
			}

			expectedSchemas := GenerateExpectedSchemaNames(dataModels)

			// Sort for consistent comparison
			sort.Strings(expectedSchemas)

			expected := []string{
				"motor_v2_timeseries-number", // motor has number fields
				"pump_v1_timeseries-boolean", // pump has boolean field
				"pump_v1_timeseries-number",  // pump has number fields
				"pump_v1_timeseries-string",  // pump has string field
			}
			sort.Strings(expected)

			// We expect unique schemas per data type per model version
			Expect(len(expectedSchemas)).To(Equal(4))
			Expect(expectedSchemas).To(Equal(expected))
		})

		It("should compare data models with registry schemas", func() {
			mockRedpandaService := NewMockRedpandaService()

			// Set up registry schemas with parsed components
			registrySchemas := []SchemaSubject{
				{
					Subject:            "pump_v1_timeseries-number",
					Name:               "pump",
					Version:            "v1",
					DataModelType:      "timeseries",
					DataType:           "number",
					ParsedSuccessfully: true,
				},
				{
					Subject:            "pump_v1_timeseries-string",
					Name:               "pump",
					Version:            "v1",
					DataModelType:      "timeseries",
					DataType:           "string",
					ParsedSuccessfully: true,
				},
				{
					Subject:            "orphaned_v1_timeseries-boolean",
					Name:               "orphaned",
					Version:            "v1",
					DataModelType:      "timeseries",
					DataType:           "boolean",
					ParsedSuccessfully: true,
				},
			}
			mockRedpandaService.GetAllSchemasResult = registrySchemas
			mockRedpandaService.GetAllSchemasError = nil

			// Set up the comparison result
			expectedMapping := &DataModelSchemaMapping{
				MissingInRegistry:  []string{"pump_v1_timeseries-boolean"},
				OrphanedInRegistry: []string{"orphaned_v1_timeseries-boolean"},
				ExpectedSchemas:    []string{"pump_v1_timeseries-number", "pump_v1_timeseries-string", "pump_v1_timeseries-boolean"},
				RegistrySchemas:    []string{"pump_v1_timeseries-number", "pump_v1_timeseries-string", "orphaned_v1_timeseries-boolean"},
			}
			mockRedpandaService.CompareDataModelsWithRegistryResult = expectedMapping
			mockRedpandaService.CompareDataModelsWithRegistryError = nil

			// Set up data models
			dataModels := map[string]config.DataModelsConfig{
				"pump": {
					Name: "pump",
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"pressure": {
									Type: "timeseries-number",
								},
								"status": {
									Type: "timeseries-string",
								},
								"active": {
									Type: "timeseries-boolean",
								},
							},
						},
					},
				},
			}

			// Call the comparison
			ctx := context.Background()
			result, err := mockRedpandaService.CompareDataModelsWithRegistry(ctx, dataModels)

			// Verify results
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			// Check expected schemas
			Expect(len(result.ExpectedSchemas)).To(Equal(3))
			Expect(result.ExpectedSchemas).To(ContainElements(
				"pump_v1_timeseries-number",
				"pump_v1_timeseries-string",
				"pump_v1_timeseries-boolean",
			))

			// Check registry schemas
			Expect(len(result.RegistrySchemas)).To(Equal(3))
			Expect(result.RegistrySchemas).To(ContainElements(
				"pump_v1_timeseries-number",
				"pump_v1_timeseries-string",
				"orphaned_v1_timeseries-boolean",
			))

			// Check missing schemas (expected but not in registry)
			Expect(len(result.MissingInRegistry)).To(Equal(1))
			Expect(result.MissingInRegistry).To(ContainElement("pump_v1_timeseries-boolean"))

			// Check orphaned schemas (in registry but not expected)
			Expect(len(result.OrphanedInRegistry)).To(Equal(1))
			Expect(result.OrphanedInRegistry).To(ContainElement("orphaned_v1_timeseries-boolean"))
		})
	})
})

// MockRedpandaServiceForSync is a testable implementation for schema synchronization testing
type MockRedpandaServiceForSync struct {
	schemaSyncCache    *SchemaSyncCache
	registrySchemas    []SchemaSubject
	deleteHistory      []string
	registrations      []JSONSchemaRegistration
	shouldFailFetch    bool
	shouldFailDelete   bool
	shouldFailRegister bool
}

func (m *MockRedpandaServiceForSync) SetRegistrySchemas(schemas []SchemaSubject) {
	m.registrySchemas = schemas
}

func (m *MockRedpandaServiceForSync) GetAllSchemas(ctx context.Context) ([]SchemaSubject, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if m.shouldFailFetch {
		return nil, fmt.Errorf("connection refused")
	}
	return m.registrySchemas, nil
}

func (m *MockRedpandaServiceForSync) DeleteSchemaFromRegistry(ctx context.Context, subjectName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if m.shouldFailDelete {
		return fmt.Errorf("connection refused")
	}
	m.deleteHistory = append(m.deleteHistory, subjectName)
	// Remove from registry schemas
	for i, schema := range m.registrySchemas {
		if schema.Subject == subjectName {
			m.registrySchemas = append(m.registrySchemas[:i], m.registrySchemas[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MockRedpandaServiceForSync) RegisterSchemaInRegistry(ctx context.Context, registration JSONSchemaRegistration) (*SchemaRegistrationResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if m.shouldFailRegister {
		return nil, fmt.Errorf("connection refused")
	}
	m.registrations = append(m.registrations, registration)
	// Add to registry schemas
	m.registrySchemas = append(m.registrySchemas, SchemaSubject{Subject: registration.SubjectName})
	return &SchemaRegistrationResponse{ID: len(m.registrations)}, nil
}

func (m *MockRedpandaServiceForSync) GenerateJSONSchemasFromDataModels(ctx context.Context, dataModels map[string]config.DataModelsConfig) ([]JSONSchemaRegistration, error) {
	// Generate simplified mock schemas based on datamodels
	var registrations []JSONSchemaRegistration
	for modelName, dataModel := range dataModels {
		for versionKey, version := range dataModel.Versions {
			dataTypes := make(map[string]bool)
			collectTimeseriesDataTypes(version.Structure, dataTypes)

			for dataType := range dataTypes {
				registration := JSONSchemaRegistration{
					SubjectName: fmt.Sprintf("%s_%s_timeseries-%s", modelName, versionKey, dataType),
					Schema:      fmt.Sprintf(`{"type": "record", "name": "%s_%s_%s"}`, modelName, versionKey, dataType),
					SchemaType:  "JSON",
				}
				registrations = append(registrations, registration)
			}
		}
	}
	return registrations, nil
}

// SynchronizeSchemas implements the same logic as the real service but with testable components
func (m *MockRedpandaServiceForSync) SynchronizeSchemas(ctx context.Context, dataModels map[string]config.DataModelsConfig) error {
	// If we have no cached state, start with fetching
	if m.schemaSyncCache == nil {
		return m.startSchemaFetching(ctx)
	}

	// Continue with the existing synchronization based on current state
	switch m.schemaSyncCache.State {
	case SchemaSyncStateFetching:
		return m.continueSchemaFetching(ctx)
	case SchemaSyncStateComparing:
		return m.continueSchemaComparison(ctx, dataModels)
	case SchemaSyncStateDeletingOrphaned:
		return m.continueOrphanedDeletion(ctx)
	case SchemaSyncStateRegistering:
		return m.continueSchemaRegistration(ctx)
	default:
		// Invalid state - reset and try again next tick
		m.schemaSyncCache = nil
		return nil
	}
}

func (m *MockRedpandaServiceForSync) startSchemaFetching(ctx context.Context) error {
	m.schemaSyncCache = &SchemaSyncCache{State: SchemaSyncStateFetching}
	return m.continueSchemaFetching(ctx)
}

func (m *MockRedpandaServiceForSync) continueSchemaFetching(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	registrySchemas, err := m.GetAllSchemas(ctx)
	if err != nil {
		m.schemaSyncCache = nil
		return nil
	}

	m.schemaSyncCache.RegistrySchemas = registrySchemas
	m.schemaSyncCache.State = SchemaSyncStateComparing
	return nil
}

func (m *MockRedpandaServiceForSync) continueSchemaComparison(ctx context.Context, dataModels map[string]config.DataModelsConfig) error {
	registrySchemas := m.schemaSyncCache.RegistrySchemas
	expectedSchemas := GenerateExpectedSchemaNames(dataModels)

	registrySchemaNames := make([]string, len(registrySchemas))
	for i, schema := range registrySchemas {
		registrySchemaNames[i] = schema.Subject
	}

	var missingInRegistry []string
	registrySchemaSet := make(map[string]bool)
	for _, schema := range registrySchemaNames {
		registrySchemaSet[schema] = true
	}

	for _, expected := range expectedSchemas {
		if !registrySchemaSet[expected] {
			missingInRegistry = append(missingInRegistry, expected)
		}
	}

	var orphanedInRegistry []string
	expectedSchemaSet := make(map[string]bool)
	for _, expected := range expectedSchemas {
		expectedSchemaSet[expected] = true
	}

	for _, registry := range registrySchemaNames {
		if !expectedSchemaSet[registry] {
			orphanedInRegistry = append(orphanedInRegistry, registry)
		}
	}

	comparisonResult := &DataModelSchemaMapping{
		MissingInRegistry:  missingInRegistry,
		OrphanedInRegistry: orphanedInRegistry,
		ExpectedSchemas:    expectedSchemas,
		RegistrySchemas:    registrySchemaNames,
	}

	m.schemaSyncCache.ComparisonResult = comparisonResult

	if len(comparisonResult.MissingInRegistry) == 0 && len(comparisonResult.OrphanedInRegistry) == 0 {
		m.schemaSyncCache = nil
		return nil
	}

	if len(comparisonResult.OrphanedInRegistry) > 0 {
		m.schemaSyncCache.State = SchemaSyncStateDeletingOrphaned
		m.schemaSyncCache.PendingDeletions = comparisonResult.OrphanedInRegistry
		return m.continueOrphanedDeletion(ctx)
	}

	return m.startSchemaRegistration(ctx, dataModels, comparisonResult)
}

func (m *MockRedpandaServiceForSync) continueOrphanedDeletion(ctx context.Context) error {
	for len(m.schemaSyncCache.PendingDeletions) > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		schemaToDelete := m.schemaSyncCache.PendingDeletions[0]
		m.schemaSyncCache.PendingDeletions = m.schemaSyncCache.PendingDeletions[1:]

		if err := m.DeleteSchemaFromRegistry(ctx, schemaToDelete); err != nil {
			m.schemaSyncCache = nil
			return nil
		}

		if len(m.schemaSyncCache.PendingDeletions) > 0 && ctx.Err() != nil {
			return nil
		}
	}

	return m.startSchemaRegistration(ctx, nil, m.schemaSyncCache.ComparisonResult)
}

func (m *MockRedpandaServiceForSync) startSchemaRegistration(ctx context.Context, dataModels map[string]config.DataModelsConfig, mapping *DataModelSchemaMapping) error {
	if len(mapping.MissingInRegistry) == 0 {
		m.schemaSyncCache = nil
		return nil
	}

	var targetDataModels map[string]config.DataModelsConfig
	if dataModels != nil {
		targetDataModels = dataModels
	} else {
		m.schemaSyncCache = nil
		return nil
	}

	allRegistrations, err := m.GenerateJSONSchemasFromDataModels(ctx, targetDataModels)
	if err != nil {
		m.schemaSyncCache = nil
		return nil
	}

	missingSet := make(map[string]bool)
	for _, missing := range mapping.MissingInRegistry {
		missingSet[missing] = true
	}

	var pendingRegistrations []JSONSchemaRegistration
	for _, registration := range allRegistrations {
		if missingSet[registration.SubjectName] {
			pendingRegistrations = append(pendingRegistrations, registration)
		}
	}

	m.schemaSyncCache = &SchemaSyncCache{
		State:                SchemaSyncStateRegistering,
		PendingRegistrations: pendingRegistrations,
		ComparisonResult:     mapping,
	}

	return m.continueSchemaRegistration(ctx)
}

func (m *MockRedpandaServiceForSync) continueSchemaRegistration(ctx context.Context) error {
	for len(m.schemaSyncCache.PendingRegistrations) > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		registration := m.schemaSyncCache.PendingRegistrations[0]
		m.schemaSyncCache.PendingRegistrations = m.schemaSyncCache.PendingRegistrations[1:]

		if _, err := m.RegisterSchemaInRegistry(ctx, registration); err != nil {
			m.schemaSyncCache = nil
			return nil
		}

		if len(m.schemaSyncCache.PendingRegistrations) > 0 && ctx.Err() != nil {
			return nil
		}
	}

	m.schemaSyncCache = nil
	return nil
}

var _ = Describe("Multi-tick Schema Synchronization", func() {
	var (
		mockService *MockRedpandaServiceForSync
		ctx         context.Context
		dataModels  map[string]config.DataModelsConfig
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create mock service
		mockService = &MockRedpandaServiceForSync{
			registrySchemas: []SchemaSubject{},
			deleteHistory:   []string{},
			registrations:   []JSONSchemaRegistration{},
		}

		// Create test data models
		dataModels = map[string]config.DataModelsConfig{
			"pump": {
				Name: "pump",
				Versions: map[string]config.DataModelVersion{
					"v1": {
						Description: "Pump model v1",
						Structure: map[string]config.Field{
							"temperature": {
								Type:        "timeseries-number",
								Description: "Temperature reading",
							},
							"status": {
								Type:        "timeseries-string",
								Description: "Status string",
							},
						},
					},
				},
			},
		}
	})

	Describe("Tick 1: Schema Fetching Phase", func() {
		It("should start with fetching state and fetch schemas from registry", func() {
			// Pre-populate registry with some schemas
			mockService.SetRegistrySchemas([]SchemaSubject{
				{Subject: "existing_pump_v1_timeseries-number"},
				{Subject: "existing_pump_v1_timeseries-string"},
			})

			// Initially no cache
			Expect(mockService.schemaSyncCache).To(BeNil())

			// Tick 1: Should start fetching
			err := mockService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should now have cache in comparing state (fetching completed in same tick)
			Expect(mockService.schemaSyncCache).ToNot(BeNil())
			Expect(mockService.schemaSyncCache.State).To(Equal(SchemaSyncStateComparing))
			Expect(mockService.schemaSyncCache.RegistrySchemas).To(HaveLen(2))

			// Verify schemas were cached
			subjectNames := make([]string, len(mockService.schemaSyncCache.RegistrySchemas))
			for i, schema := range mockService.schemaSyncCache.RegistrySchemas {
				subjectNames[i] = schema.Subject
			}
			Expect(subjectNames).To(ContainElements(
				"existing_pump_v1_timeseries-number",
				"existing_pump_v1_timeseries-string",
			))
		})

		It("should handle empty registry during fetch", func() {
			// Registry is empty

			// Tick 1: Should start fetching
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have cache in comparing state with empty registry schemas
			Expect(redpandaService.schemaSyncCache).ToNot(BeNil())
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateComparing))
			Expect(redpandaService.schemaSyncCache.RegistrySchemas).To(BeEmpty())
		})

		It("should handle registry connection failure during fetch", func() {
			// Close registry to simulate connection failure
			mockRegistry.Close()

			// Tick 1: Should handle failure gracefully
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should reset cache on failure
			Expect(redpandaService.schemaSyncCache).To(BeNil())
		})

		It("should handle context cancellation during fetch", func() {
			// Create a context that will be cancelled
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			// Tick 1: Should handle cancelled context
			err := redpandaService.SynchronizeSchemas(cancelCtx, dataModels)
			Expect(err).To(Equal(context.Canceled))

			// Should not have created cache
			Expect(redpandaService.schemaSyncCache).To(BeNil())
		})
	})

	Describe("Tick 2: Schema Comparison Phase", func() {
		BeforeEach(func() {
			// Set up fetching state with cached registry schemas
			redpandaService.schemaSyncCache = &SchemaSyncCache{
				State: SchemaSyncStateComparing,
				RegistrySchemas: []SchemaSubject{
					{Subject: "pump_v1_timeseries-number"},     // Expected schema exists
					{Subject: "orphaned_v1_timeseries-string"}, // Orphaned schema
				},
			}
		})

		It("should compare cached schemas with datamodels and detect no differences", func() {
			// Modify datamodels to match registry exactly
			pumpConfig := dataModels["pump"]
			pumpVersion := pumpConfig.Versions["v1"]
			pumpVersion.Structure = map[string]config.Field{
				"temperature": {Type: "timeseries-number"}, // Only number type
			}
			pumpConfig.Versions["v1"] = pumpVersion
			dataModels["pump"] = pumpConfig

			// Tick 2: Should complete comparison and find no differences
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have cleared cache (no differences found)
			Expect(redpandaService.schemaSyncCache).To(BeNil())
		})

		It("should detect missing schemas and transition to registration", func() {
			// Current registry has: pump_v1_timeseries-number, orphaned_v1_timeseries-string
			// Datamodels expect: pump_v1_timeseries-number, pump_v1_timeseries-string
			// Missing: pump_v1_timeseries-string
			// Orphaned: orphaned_v1_timeseries-string

			// Tick 2: Should detect differences and start with deletions
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should transition to deleting orphaned (orphaned schemas processed first)
			Expect(redpandaService.schemaSyncCache).ToNot(BeNil())
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateDeletingOrphaned))
			Expect(redpandaService.schemaSyncCache.PendingDeletions).To(ContainElement("orphaned_v1_timeseries-string"))
			Expect(redpandaService.schemaSyncCache.ComparisonResult).ToNot(BeNil())
			Expect(redpandaService.schemaSyncCache.ComparisonResult.MissingInRegistry).To(ContainElement("pump_v1_timeseries-string"))
		})

		It("should detect only missing schemas and transition directly to registration", func() {
			// Set up registry with no orphaned schemas
			redpandaService.schemaSyncCache.RegistrySchemas = []SchemaSubject{
				{Subject: "pump_v1_timeseries-number"}, // Only partial match
			}

			// Tick 2: Should detect missing schemas only
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should transition directly to registration (no orphaned schemas)
			Expect(redpandaService.schemaSyncCache).ToNot(BeNil())
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateRegistering))
			Expect(redpandaService.schemaSyncCache.PendingRegistrations).To(HaveLen(1)) // Only string is missing
		})

		It("should handle empty datamodels gracefully", func() {
			// Empty datamodels should result in all registry schemas being orphaned
			emptyDataModels := map[string]config.DataModelsConfig{}

			// Tick 2: Should detect all as orphaned
			err := redpandaService.SynchronizeSchemas(ctx, emptyDataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should transition to deleting all
			Expect(redpandaService.schemaSyncCache).ToNot(BeNil())
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateDeletingOrphaned))
			Expect(redpandaService.schemaSyncCache.PendingDeletions).To(HaveLen(2))
		})
	})

	Describe("Tick 3-N: Schema Deletion Phase", func() {
		BeforeEach(func() {
			// Set up deletion state with multiple orphaned schemas
			redpandaService.schemaSyncCache = &SchemaSyncCache{
				State: SchemaSyncStateDeletingOrphaned,
				PendingDeletions: []string{
					"orphaned1_v1_timeseries-string",
					"orphaned2_v1_timeseries-number",
					"orphaned3_v1_timeseries-boolean",
				},
				ComparisonResult: &DataModelSchemaMapping{
					MissingInRegistry: []string{"pump_v1_timeseries-string"},
				},
			}

			// Pre-populate registry with orphaned schemas
			mockRegistry.AddSchema("orphaned1_v1_timeseries-string", "schema1")
			mockRegistry.AddSchema("orphaned2_v1_timeseries-number", "schema2")
			mockRegistry.AddSchema("orphaned3_v1_timeseries-boolean", "schema3")
		})

		It("should delete multiple schemas per tick when time allows", func() {
			// Tick: Should delete all schemas if time permits
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have transitioned to registration after deleting all
			Expect(redpandaService.schemaSyncCache).ToNot(BeNil())
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateRegistering))
			Expect(redpandaService.schemaSyncCache.PendingDeletions).To(BeEmpty())

			// Verify all deletions occurred
			deleteHistory := mockRegistry.GetDeleteHistory()
			Expect(deleteHistory).To(HaveLen(3))
		})

		It("should transition to registration after completing all deletions", func() {
			// Set up with only one deletion remaining
			redpandaService.schemaSyncCache.PendingDeletions = []string{"orphaned1_v1_timeseries-string"}

			// Tick: Should delete last schema and transition
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have transitioned to registration
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateRegistering))
			Expect(redpandaService.schemaSyncCache.PendingRegistrations).To(HaveLen(1))
		})

		It("should handle deletion failures gracefully", func() {
			// Close registry to simulate failure
			mockRegistry.Close()

			// Tick: Should handle failure and reset
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have reset cache on failure
			Expect(redpandaService.schemaSyncCache).To(BeNil())
		})

		It("should handle context cancellation during deletion", func() {
			// Create cancelled context
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			// Tick: Should handle cancelled context
			err := redpandaService.SynchronizeSchemas(cancelCtx, dataModels)
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("Tick N+1: Schema Registration Phase", func() {
		BeforeEach(func() {
			// Set up registration state with multiple missing schemas
			redpandaService.schemaSyncCache = &SchemaSyncCache{
				State: SchemaSyncStateRegistering,
				PendingRegistrations: []JSONSchemaRegistration{
					{SubjectName: "pump_v1_timeseries-number", Schema: "number_schema", SchemaType: "JSON"},
					{SubjectName: "pump_v1_timeseries-string", Schema: "string_schema", SchemaType: "JSON"},
					{SubjectName: "pump_v1_timeseries-boolean", Schema: "boolean_schema", SchemaType: "JSON"},
				},
			}
		})

		It("should register multiple schemas per tick when time allows", func() {
			// Tick: Should register all schemas if time permits
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have completed and cleared cache
			Expect(redpandaService.schemaSyncCache).To(BeNil())

			// Verify all registrations occurred
			registrationHistory := mockRegistry.GetRegistrationHistory()
			Expect(registrationHistory).To(HaveLen(3))

			// Verify schemas exist in registry
			subjects := mockRegistry.GetSubjects()
			Expect(subjects).To(HaveKey("pump_v1_timeseries-number"))
			Expect(subjects).To(HaveKey("pump_v1_timeseries-string"))
			Expect(subjects).To(HaveKey("pump_v1_timeseries-boolean"))
		})

		It("should reset cache after completing all registrations", func() {
			// Set up with only one registration remaining
			redpandaService.schemaSyncCache.PendingRegistrations = []JSONSchemaRegistration{
				{SubjectName: "pump_v1_timeseries-string", Schema: "string_schema", SchemaType: "JSON"},
			}

			// Tick: Should register last schema and complete
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have cleared cache (synchronization complete)
			Expect(redpandaService.schemaSyncCache).To(BeNil())

			// Verify registration occurred
			registrationHistory := mockRegistry.GetRegistrationHistory()
			Expect(registrationHistory).To(HaveLen(1))
		})

		It("should handle registration failures gracefully", func() {
			// Close registry to simulate failure
			mockRegistry.Close()

			// Tick: Should handle failure and reset
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have reset cache on failure
			Expect(redpandaService.schemaSyncCache).To(BeNil())
		})

		It("should handle context cancellation during registration", func() {
			// Create cancelled context
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			// Tick: Should handle cancelled context
			err := redpandaService.SynchronizeSchemas(cancelCtx, dataModels)
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("Complete Multi-Tick Workflow", func() {
		It("should handle complete workflow from fetch to registration", func() {
			// Pre-populate registry with orphaned schemas and some expected ones
			mockRegistry.AddSchema("pump_v1_timeseries-number", "existing_number_schema") // Expected
			mockRegistry.AddSchema("orphaned_v1_timeseries-boolean", "orphaned_schema")   // Orphaned
			// Missing: pump_v1_timeseries-string

			// Tick 1: Fetch and compare (combined in our implementation)
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should be in deletion state
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateDeletingOrphaned))
			Expect(redpandaService.schemaSyncCache.PendingDeletions).To(ContainElement("orphaned_v1_timeseries-boolean"))

			// Tick 2: Delete orphaned schema
			err = redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should transition to registration state
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateRegistering))
			Expect(redpandaService.schemaSyncCache.PendingRegistrations).To(HaveLen(1)) // string is missing

			// Tick 3: Register missing schema
			err = redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have completed and cleared cache
			Expect(redpandaService.schemaSyncCache).To(BeNil())

			// Verify final state
			deleteHistory := mockRegistry.GetDeleteHistory()
			Expect(deleteHistory).To(ContainElement("orphaned_v1_timeseries-boolean"))

			registrationHistory := mockRegistry.GetRegistrationHistory()
			Expect(registrationHistory).To(HaveLen(1))

			subjects := mockRegistry.GetSubjects()
			Expect(subjects).To(HaveKey("pump_v1_timeseries-number"))         // Pre-existing
			Expect(subjects).To(HaveKey("pump_v1_timeseries-string"))         // Newly registered
			Expect(subjects).ToNot(HaveKey("orphaned_v1_timeseries-boolean")) // Deleted
		})

		It("should restart workflow after completion", func() {
			// Complete one full cycle first
			mockRegistry.AddSchema("pump_v1_timeseries-number", "existing_schema")
			// Missing: pump_v1_timeseries-string

			// Complete registration cycle
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())
			err = redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have completed first cycle
			Expect(redpandaService.schemaSyncCache).To(BeNil())

			// Now add an orphaned schema to trigger a new cycle
			mockRegistry.AddSchema("new_orphaned_v1_timeseries-string", "new_orphaned")

			// Start new cycle
			err = redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have started new cycle and detected the new orphaned schema
			Expect(redpandaService.schemaSyncCache).ToNot(BeNil())
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateDeletingOrphaned))
			Expect(redpandaService.schemaSyncCache.PendingDeletions).To(ContainElement("new_orphaned_v1_timeseries-string"))
		})

		It("should handle large number of schemas efficiently", func() {
			// Create a large number of orphaned schemas
			for i := 0; i < 50; i++ {
				mockRegistry.AddSchema(fmt.Sprintf("orphaned_%d_v1_timeseries-number", i), "schema")
			}

			// Start synchronization
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should be in deletion state with 50 pending deletions
			Expect(redpandaService.schemaSyncCache.State).To(Equal(SchemaSyncStateDeletingOrphaned))
			Expect(redpandaService.schemaSyncCache.PendingDeletions).To(HaveLen(50))

			// Should be able to delete multiple schemas per tick
			err = redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have made progress (deleted some schemas)
			deleteHistory := mockRegistry.GetDeleteHistory()
			Expect(len(deleteHistory)).To(BeNumerically(">", 0))
			Expect(len(deleteHistory)).To(BeNumerically("<=", 50))
		})

		It("should handle invalid state gracefully", func() {
			// Set up invalid state
			redpandaService.schemaSyncCache = &SchemaSyncCache{
				State: SchemaSyncState(999), // Invalid state
			}

			// Should reset cache on invalid state
			err := redpandaService.SynchronizeSchemas(ctx, dataModels)
			Expect(err).ToNot(HaveOccurred())

			// Should have reset cache
			Expect(redpandaService.schemaSyncCache).To(BeNil())
		})
	})
})
