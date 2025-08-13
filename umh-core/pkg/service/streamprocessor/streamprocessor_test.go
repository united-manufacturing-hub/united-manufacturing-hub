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

package streamprocessor

import (
	"context"
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dfcservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	runtime_config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor/runtime_config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("StreamProcessorService", func() {
	var (
		service         *Service
		mockDfc         *dfcservice.MockDataFlowComponentService
		ctx             context.Context
		spName          string
		cancelFunc      context.CancelFunc
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx, cancelFunc = context.WithDeadline(context.Background(), time.Now().Add(500*time.Second))
		spName = "test-streamprocessor"

		// Set up mock DFC service
		mockDfc = dfcservice.NewMockDataFlowComponentService()

		// Set up a real service with mocked dependencies
		service = NewDefaultService(spName, WithUnderlyingService(mockDfc))
		mockSvcRegistry = serviceregistry.NewMockRegistry()
	})

	AfterEach(func() {
		// Clean up if necessary
		cancelFunc()
	})

	Describe("AddToManager", func() {
		var (
			cfg       streamprocessorserviceconfig.StreamProcessorServiceConfigSpec
			dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
		)

		BeforeEach(func() {
			// Create a realistic streamprocessor config using location-based templating
			cfg = streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "pump",
						Version: "v1",
					},
					Sources: map[string]string{
						"vibration_sensor": "{{ .location_path }}.vibration",
						"count_sensor":     "{{ .location_path }}.count",
						"motor_data":       "{{ .location_path }}.motor",
					},
					Mapping: map[string]any{
						"count":     "count_sensor.value",
						"vibration": "vibration_sensor",
						"motor": map[string]any{
							"speed":       "motor_data.speed",
							"current":     "motor_data.current",
							"temperature": "motor_data.temperature",
							"status":      "motor_data.status",
						},
					},
				},
			}

			// Set up agent location only
			agentLocation := map[string]string{
				"0": "factory-A",
				"1": "line-1",
			}

			var err error
			_, dfcConfig, err = runtime_config.BuildRuntimeConfig(cfg, agentLocation, nil, "test-node", spName)
			Expect(err).NotTo(HaveOccurred())

			// Set up mock to return false for service exists
			mockDfc.ServiceExistsResult = false
		})

		It("should add a new streamprocessor to the underlying DFC manager", func() {
			// Act
			err := service.AddToManager(ctx, mockSvcRegistry.GetFileSystem(), &dfcConfig, spName)

			// Assert
			Expect(err).NotTo(HaveOccurred())

			// Verify that a config was added to the service
			Expect(service.dataflowComponentConfig).To(HaveLen(1))

			// Verify the name follows the expected pattern
			underlyingDFCName := service.getUnderlyingDFCReadName(spName)
			Expect(service.dataflowComponentConfig[0].Name).To(Equal(underlyingDFCName))

			// Verify the desired state is set correctly
			Expect(service.dataflowComponentConfig[0].DesiredFSMState).To(Equal(dfcfsm.OperationalStateStopped))
		})

		It("should create DFC config with proper UNS structure", func() {
			// Verify the DFC config has the expected structure
			Expect(dfcConfig.BenthosConfig.Input).To(HaveKey("uns"))
			Expect(dfcConfig.BenthosConfig.Output).To(HaveKey("uns"))
			Expect(dfcConfig.BenthosConfig.Pipeline).To(HaveKey("processors"))

			// Check UNS input structure
			unsInput, ok := dfcConfig.BenthosConfig.Input["uns"].(map[string]any)
			Expect(ok).To(BeTrue(), "UNS input should be a map[string]any")
			Expect(unsInput).To(HaveKey("umh_topics"))

			umhTopics, ok := unsInput["umh_topics"].([]any)
			Expect(ok).To(BeTrue(), "umh_topics should be a []any")
			Expect(umhTopics).To(ContainElement("umh.v1.factory-A.line-1.vibration"))
			Expect(umhTopics).To(ContainElement("umh.v1.factory-A.line-1.count"))
			Expect(umhTopics).To(ContainElement("umh.v1.factory-A.line-1.motor"))

			// Check stream processor pipeline structure
			processors, ok := dfcConfig.BenthosConfig.Pipeline["processors"].([]any)
			if ok {
				processor, ok := processors[0].(map[string]any)
				if ok {
					Expect(processor).To(HaveKey("stream_processor"))
					streamprocessor, ok := processor["stream_processor"].(map[string]any)
					if ok {
						Expect(streamprocessor).To(HaveKey("model"))
						Expect(streamprocessor).To(HaveKey("sources"))
						Expect(streamprocessor).To(HaveKey("mapping"))
						Expect(streamprocessor).To(HaveKey("output_topic"))
						// Verify model
						model, ok := streamprocessor["model"].(map[string]any)
						Expect(ok).To(BeTrue(), "model should be a map[string]any")
						Expect(model["name"]).To(Equal("pump"))
						Expect(model["version"]).To(Equal("v1"))

						// Verify output topic
						Expect(streamprocessor["output_topic"]).To(Equal("umh.v1.factory-A.line-1"))

						// Verify sources mapping
						sources, ok := streamprocessor["sources"].(map[string]any)
						Expect(ok).To(BeTrue(), "sources should be a map[string]any")
						Expect(sources["vibration_sensor"]).To(Equal("umh.v1.factory-A.line-1.vibration"))
						Expect(sources["count_sensor"]).To(Equal("umh.v1.factory-A.line-1.count"))
						Expect(sources["motor_data"]).To(Equal("umh.v1.factory-A.line-1.motor"))
					}
				}
				Expect(ok).To(BeTrue())
			}
			Expect(ok).To(BeTrue())
		})

		It("should return error when the streamprocessor already exists", func() {
			// Add the StreamProcessor first
			err := service.AddToManager(ctx, mockSvcRegistry.GetFileSystem(), &dfcConfig, spName)
			Expect(err).NotTo(HaveOccurred())

			// Try to add it again
			err = service.AddToManager(ctx, mockSvcRegistry.GetFileSystem(), &dfcConfig, spName)

			// Assert
			Expect(err).To(MatchError(ErrServiceAlreadyExists))
		})
	})

	Describe("GetConfig", func() {
		var (
			cfg       streamprocessorserviceconfig.StreamProcessorServiceConfigSpec
			dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
		)

		BeforeEach(func() {
			// Create a config for testing using location-based templating
			cfg = streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "motor",
						Version: "v1",
					},
					Sources: map[string]string{
						"motor_telemetry": "{{ .location_path }}.motor.telemetry",
					},
					Mapping: map[string]any{
						"speed":       "motor_telemetry.speed",
						"current":     "motor_telemetry.current",
						"temperature": "motor_telemetry.temperature",
						"status":      "operational",
					},
				},
			}

			// Set up agent location only
			agentLocation := map[string]string{
				"0": "factory-A",
				"1": "line-1",
			}

			// Build runtime config using BuildRuntimeConfig
			var err error
			_, dfcConfig, err = runtime_config.BuildRuntimeConfig(cfg, agentLocation, nil, "test-node", spName)
			Expect(err).NotTo(HaveOccurred())

			// Add the component to the service first
			err = service.AddToManager(ctx, mockSvcRegistry.GetFileSystem(), &dfcConfig, spName)
			Expect(err).NotTo(HaveOccurred())

			// Set up the mock DFC service to return the config we expect
			mockDfc.GetConfigResult = dfcConfig
		})

		It("should retrieve the streamprocessor config by converting from DFC config", func() {
			// Act
			result, err := service.GetConfig(ctx, mockSvcRegistry.GetFileSystem(), spName)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(mockDfc.GetConfigCalled).To(BeTrue())

			// Verify the config was properly converted from DFC format
			Expect(result.Model.Name).To(Equal("motor"))
			Expect(result.Model.Version).To(Equal("v1"))

			// Check that templates were rendered (location_path should be resolved)
			Expect(result.Sources["motor_telemetry"]).To(Equal("factory-A.line-1.motor.telemetry"))
		})

		It("should return error when service doesn't exist", func() {
			// Configure mock to simulate service not existing
			mockDfc.GetConfigError = ErrServiceNotExist

			// Act
			_, err := service.GetConfig(ctx, mockSvcRegistry.GetFileSystem(), "non-existent")

			// Assert
			Expect(err).To(MatchError(ContainSubstring("failed to get read dataflowcomponent config")))
		})
	})

	Describe("UpdateInManager", func() {
		var (
			initialConfig    streamprocessorserviceconfig.StreamProcessorServiceConfigSpec
			updatedConfig    streamprocessorserviceconfig.StreamProcessorServiceConfigSpec
			initialDFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
			updatedDFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
		)

		BeforeEach(func() {
			// Initial config
			initialConfig = streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "pump",
						Version: "v1",
					},
					Sources: map[string]string{
						"vibration_sensor": "{{ .location_path }}.vibration",
						"count_sensor":     "{{ .location_path }}.count",
					},
					Mapping: map[string]any{
						"count":     "count_sensor.value",
						"vibration": "vibration_sensor",
					},
				},
			}

			// Updated config with added motor data
			updatedConfig = streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "pump",
						Version: "v1",
					},
					Sources: map[string]string{
						"vibration_sensor": "{{ .location_path }}.vibration",
						"count_sensor":     "{{ .location_path }}.count",
						"motor_data":       "{{ .location_path }}.motor", // Added motor data
					},
					Mapping: map[string]any{
						"count":     "count_sensor.value",
						"vibration": "vibration_sensor",
						"motor": map[string]any{
							"speed": "motor_data.speed",
						},
					},
				},
			}

			// Set up agent location only
			agentLocation := map[string]string{
				"0": "factory-A",
				"1": "line-1",
			}

			// Build runtime configs using BuildRuntimeConfig
			var err error
			_, initialDFCConfig, err = runtime_config.BuildRuntimeConfig(initialConfig, agentLocation, nil, "test-node", spName)
			Expect(err).NotTo(HaveOccurred())

			_, updatedDFCConfig, err = runtime_config.BuildRuntimeConfig(updatedConfig, agentLocation, nil, "test-node", spName)
			Expect(err).NotTo(HaveOccurred())

			// Add the component first
			err = service.AddToManager(ctx, mockSvcRegistry.GetFileSystem(), &initialDFCConfig, spName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update an existing streamprocessor configuration", func() {
			// Act
			err := service.UpdateInManager(ctx, mockSvcRegistry.GetFileSystem(), &updatedDFCConfig, spName)

			// Assert
			Expect(err).NotTo(HaveOccurred())

			// Verify the service was updated
			Expect(service.dataflowComponentConfig).To(HaveLen(1))

			// The configuration should have been updated with the new DFC config
			underlyingDFCName := service.getUnderlyingDFCReadName(spName)
			Expect(service.dataflowComponentConfig[0].Name).To(Equal(underlyingDFCName))
		})

		It("should return error when trying to update non-existent service", func() {
			// Act
			err := service.UpdateInManager(ctx, mockSvcRegistry.GetFileSystem(), &updatedDFCConfig, "non-existent")

			// Assert
			Expect(err).To(MatchError(ErrServiceNotExist))
		})
	})

	Describe("lifecycle management", func() {
		var (
			cfg       streamprocessorserviceconfig.StreamProcessorServiceConfigSpec
			dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
		)

		BeforeEach(func() {
			// Create config using location-based templating
			cfg = streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "pump",
						Version: "v1",
					},
					Sources: map[string]string{
						"vibration_sensor": "{{ .location_path }}.vibration",
						"count_sensor":     "{{ .location_path }}.count",
					},
					Mapping: map[string]any{
						"count":     "count_sensor.value",
						"vibration": "vibration_sensor",
					},
				},
			}

			// Set up agent location only
			agentLocation := map[string]string{
				"0": "factory-A",
				"1": "line-1",
			}

			// Build runtime config using BuildRuntimeConfig
			var err error
			_, dfcConfig, err = runtime_config.BuildRuntimeConfig(cfg, agentLocation, nil, "test-node", spName)
			Expect(err).NotTo(HaveOccurred())

			// Add the component
			err = service.AddToManager(ctx, mockSvcRegistry.GetFileSystem(), &dfcConfig, spName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle start/stop/remove operations", func() {
			// Start
			err := service.Start(ctx, mockSvcRegistry.GetFileSystem(), spName)
			Expect(err).NotTo(HaveOccurred())

			// Stop
			err = service.Stop(ctx, mockSvcRegistry.GetFileSystem(), spName)
			Expect(err).NotTo(HaveOccurred())

			// Remove
			err = service.RemoveFromManager(ctx, mockSvcRegistry.GetFileSystem(), spName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the service was removed
			Expect(service.dataflowComponentConfig).To(BeEmpty())
		})
	})

	Describe("runtime_config.BuildRuntimeConfig", func() {
		It("should successfully build runtime config from example config", func() {
			// Parse the example config file using the config manager's parseConfig function
			exampleConfigPath := "../../../examples/example-config-streamprocessor-templated.yaml"

			// Read the example config file
			data, err := os.ReadFile(exampleConfigPath)
			Expect(err).NotTo(HaveOccurred(), "Failed to read example config file")

			// Use the config manager's parseConfig function to properly handle templates and anchors
			ctx := context.Background()
			fullConfig, err := config.ParseConfig(data, ctx, true) // Allow unknown fields for template handling
			Expect(err).NotTo(HaveOccurred(), "Failed to parse example config")

			// Extract the first stream processor (pump-processor)
			Expect(fullConfig.StreamProcessor).To(HaveLen(3), "Expected 3 stream processors in example config")
			firstSP := fullConfig.StreamProcessor[0]
			Expect(firstSP.Name).To(Equal("pump-processor"), "Expected first SP to be pump-processor")

			// Get the spec from the first stream processor
			spec := firstSP.StreamProcessorServiceConfig

			// Extract agent location from the config
			agentLocation := map[string]string{}
			for k, v := range fullConfig.Agent.Location {
				agentLocation[strconv.Itoa(k)] = v
			}

			// Set up test data
			globalVars := map[string]any{
				"releaseChannel": "stable",
				"version":        "1.0.0",
			}
			nodeName := "test-node"
			spName := "pump-processor"

			// Build runtime config
			spRuntimeCfg, dfcRuntimeCfg, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, globalVars, nodeName, spName)
			Expect(err).NotTo(HaveOccurred())

			// Verify the runtime config structure
			Expect(spRuntimeCfg).NotTo(BeZero())
			Expect(dfcRuntimeCfg).NotTo(BeZero())

			// Check that template variables have been rendered
			Expect(spRuntimeCfg.Model.Name).To(Equal("pump"))
			Expect(spRuntimeCfg.Model.Version).To(Equal("v1"))

			// Check that location_path template was rendered
			// The example config has both agent location (factory-A.line-1) and component location (station-7.pump-001)
			// and the template includes "umh.v1." prefix, so the rendered values should reflect this
			Expect(spRuntimeCfg.Sources["vibration_sensor"]).To(Equal("umh.v1.factory-A.line-1.station-7.pump-001.vibration"))
			Expect(spRuntimeCfg.Sources["count_sensor"]).To(Equal("umh.v1.factory-A.line-1.station-7.pump-001.count"))
			Expect(spRuntimeCfg.Sources["motor_data"]).To(Equal("umh.v1.factory-A.line-1.station-7.pump-001.motor"))

			// Check that user variables were rendered
			Expect(spRuntimeCfg.Mapping["serialNumber"]).To(Equal("ABC-12345"))

			// Verify DFC config has proper UNS structure
			Expect(dfcRuntimeCfg.BenthosConfig.Input).To(HaveKey("uns"))
			Expect(dfcRuntimeCfg.BenthosConfig.Output).To(HaveKey("uns"))
			Expect(dfcRuntimeCfg.BenthosConfig.Pipeline).To(HaveKey("processors"))
			processors, ok := dfcRuntimeCfg.BenthosConfig.Pipeline["processors"].([]any)
			if ok {
				streamprocessor, ok := processors[0].(map[string]any)
				if ok {
					Expect(streamprocessor).To(HaveKey("stream_processor"))
				}
				Expect(ok).To(BeTrue())
			}
			Expect(ok).To(BeTrue())
			// Check UNS input topics
			unsInput, ok := dfcRuntimeCfg.BenthosConfig.Input["uns"].(map[string]any)
			Expect(ok).To(BeTrue(), "uns input should be a map[string]any")
			umhTopics, ok := unsInput["umh_topics"].([]any)
			Expect(ok).To(BeTrue(), "umh_topics should be a []any")
			Expect(umhTopics).To(ContainElement("umh.v1.factory-A.line-1.station-7.pump-001.vibration"))
			Expect(umhTopics).To(ContainElement("umh.v1.factory-A.line-1.station-7.pump-001.count"))
			Expect(umhTopics).To(ContainElement("umh.v1.factory-A.line-1.station-7.pump-001.motor"))
		})

		It("should handle location-based template variable substitution", func() {
			// Create a spec with location-based templates
			spec := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "test-model",
						Version: "v1",
					},
					Sources: map[string]string{
						"sensor_data": "{{ .location_path }}.sensor",
					},
					Mapping: map[string]any{
						"id":         "{{ .internal.id }}",
						"location_0": "{{ index .location \"0\" }}",
						"location_1": "{{ index .location \"1\" }}",
						"location_2": "{{ index .location \"2\" }}",
						"value":      "sensor_data.value",
					},
				},
			}

			// Set up agent location only
			agentLocation := map[string]string{
				"0": "factory",
				"1": "line1",
			}

			// Build the runtime config
			spRuntimeCfg, dfcRuntimeCfg, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, nil, "test-node", "test-sp")
			Expect(err).NotTo(HaveOccurred())

			// Verify location_path template was rendered
			Expect(spRuntimeCfg.Sources["sensor_data"]).To(Equal("factory.line1.sensor"))

			// Verify location variables are rendered
			Expect(spRuntimeCfg.Mapping["location_0"]).To(Equal("factory"))
			Expect(spRuntimeCfg.Mapping["location_1"]).To(Equal("line1"))
			// location_2 doesn't exist when using only agent location

			// Verify internal variables are rendered
			Expect(spRuntimeCfg.Mapping["id"]).To(Equal("test-sp"))

			// Verify DFC config has proper UNS input with rendered topic
			unsInput, ok := dfcRuntimeCfg.BenthosConfig.Input["uns"].(map[string]any)
			Expect(ok).To(BeTrue(), "uns input should be a map[string]any")
			umhTopics, ok := unsInput["umh_topics"].([]any)
			Expect(ok).To(BeTrue(), "umh_topics should be a []any")
			Expect(umhTopics).To(ContainElement("umh.v1.factory.line1.sensor"))
		})

		It("should properly use agent location", func() {
			spec := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
				Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
					Model: streamprocessorserviceconfig.ModelRef{
						Name:    "test-model",
						Version: "v1",
					},
					Sources: map[string]string{
						"test_source": "{{ .location_path }}.test",
					},
					Mapping: map[string]any{
						"test": "test_source.value",
					},
				},
			}

			agentLocation := map[string]string{
				"0": "factory-from-agent",
				"1": "line-from-agent",
			}

			spRuntimeCfg, dfcRuntimeCfg, err := runtime_config.BuildRuntimeConfig(spec, agentLocation, nil, "test-node", "test-sp")
			Expect(err).NotTo(HaveOccurred())

			// The location should be properly used from agent location
			Expect(spRuntimeCfg.Sources["test_source"]).To(Equal("factory-from-agent.line-from-agent.test"))

			// Verify DFC config has the agent location in UNS topics
			unsInput := dfcRuntimeCfg.BenthosConfig.Input["uns"].(map[string]any)
			umhTopics := unsInput["umh_topics"].([]any)
			Expect(umhTopics).To(ContainElement("umh.v1.factory-from-agent.line-from-agent.test"))
		})
	})
})
