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
