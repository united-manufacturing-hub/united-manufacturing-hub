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

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const TimeToWaitForConfigRefresh = 100 * time.Millisecond

var _ = Describe("ConfigManager", func() {
	var (
		mockFS            *filesystem.MockFileSystem
		configManager     *FileConfigManager
		ctx               context.Context
		ctxWithCancelFunc context.CancelFunc
		tick              uint64
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()

		// Create a context with a timeout for cancellation tests
		ctx = context.Background()
		ctxWithCancelFunc = func() {}
		tick = uint64(0)
	})

	JustBeforeEach(func() {
		configManager = NewFileConfigManager()
		configManager.WithFileSystemService(mockFS)
	})

	AfterEach(func() {
		// Clean up resources
		ctxWithCancelFunc()
	})

	Describe("GetConfig", func() {
		var (
			validYAML = `
internal:
  services:
    - name: service1
      desiredState: running
      s6ServiceConfig:
        command: ["/bin/echo", "hello world"]
        env:
          KEY: value
        configFiles:
          file.txt: content
`
			invalidYAML = `services: - invalid: yaml: content`
		)

		Context("when file exists and contains valid YAML", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					Expect(path).To(Equal(filepath.Dir(DefaultConfigPath)))
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					Expect(path).To(Equal(DefaultConfigPath))
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					Expect(path).To(Equal(DefaultConfigPath))
					return []byte(validYAML), nil
				})
			})

			It("should return the parsed config", func() {

				// Wait for background refresh to complete and verify config
				Eventually(func() error {

					var config FullConfig
					Eventually(func() error {
						var err error
						config, err = configManager.GetConfig(ctx, tick)
						return err
					}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

					if len(config.Internal.Services) != 1 {
						return fmt.Errorf("expected 1 service, got %d", len(config.Internal.Services))
					}

					service := config.Internal.Services[0]
					if service.Name != "service1" {
						return fmt.Errorf("expected service name 'service1', got '%s'", service.Name)
					}

					if service.FSMInstanceConfig.DesiredFSMState != "running" {
						return fmt.Errorf("expected desired state 'running', got '%s'", service.FSMInstanceConfig.DesiredFSMState)
					}

					// All checks passed, verify remaining fields
					Expect(service.S6ServiceConfig.Command).To(Equal([]string{"/bin/echo", "hello world"}))
					Expect(service.S6ServiceConfig.Env).To(HaveKeyWithValue("KEY", "value"))
					Expect(service.S6ServiceConfig.ConfigFiles).To(HaveKeyWithValue("file.txt", "content"))
					return nil
				}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())
			})
		})

		Context("when file does not exist", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return false, nil
				})
			})

			It("should return an empty config", func() {
				config, err := configManager.GetConfig(ctx, tick)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("config file does not exist"))
				Expect(config.Internal.Services).To(BeEmpty())
			})
		})

		Context("when file exists but contains invalid YAML", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(invalidYAML), nil
				})
			})

			It("should return an error", func() {
				// Trigger initial config load to start background refresh
				_, _ = configManager.GetConfig(ctx, tick)

				// Wait for background refresh to complete and verify error
				Eventually(func() error {
					var err error
					var config FullConfig
					Eventually(func() error {
						config, err = configManager.GetConfig(ctx, tick)
						return err
					}, TimeToWaitForConfigRefresh*2, "10ms").Should(Not(Succeed()))

					Expect(config).To(Equal(FullConfig{}))
					return nil
				}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())
			})
		})

		Context("when EnsureDirectory fails", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return errors.New("directory creation failed")
				})
			})

			It("should return an error", func() {
				_, err := configManager.GetConfig(ctx, tick)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create config directory"))
			})
		})

		Context("when FileExists fails", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return false, errors.New("file check failed")
				})
			})

			It("should return an error", func() {
				_, err := configManager.GetConfig(ctx, tick)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("file check failed"))
			})
		})

		Context("when ReadFile fails", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return nil, errors.New("file read failed")
				})
			})

			It("should return an error", func() {
				// Trigger initial config load to start background refresh
				_, _ = configManager.GetConfig(ctx, tick)

				// Wait for background refresh to complete and verify error
				Eventually(func() error {
					_, err := configManager.GetConfig(ctx, tick)
					if err == nil {
						return fmt.Errorf("expected error but got none")
					}
					if !strings.Contains(err.Error(), "failed to read config file") {
						return fmt.Errorf("expected error to contain 'failed to read config file', got: %s", err.Error())
					}
					return nil
				}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())
			})
		})

		Context("when context is canceled", func() {
			BeforeEach(func() {
				// Create a context with cancel function
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(context.Background())
				ctxWithCancelFunc = cancel

				// Set up mock to block and check context
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					// Cancel the context
					cancel()
					// Wait a bit to ensure the cancellation propagates
					time.Sleep(10 * time.Millisecond)
					// Check if context is canceled
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
						return fmt.Errorf("context should have been canceled")
					}
				})
			})

			It("should respect context cancellation", func() {
				_, err := configManager.GetConfig(ctx, tick)
				Expect(err).To(HaveOccurred())
				// Check if the error contains context.Canceled by unwrapping it
				Expect(errors.Is(err, context.Canceled)).To(BeTrue(), "Expected error to wrap context.Canceled")
				// Also verify the error message contains the expected text
				Expect(err.Error()).To(ContainSubstring("context canceled"))
			})
		})
	})

	Describe("parseConfig", func() {
		Context("with various YAML inputs", func() {
			It("should parse valid YAML correctly", func() {
				validYAML := `
internal:
  services:
    - name: service1
      desiredState: running
  redpanda:
    desiredState: running
agent:
  metricsPort: 8080
  location:
    0: Enterprise
    1: Site
`
				config, err := ParseConfig([]byte(validYAML), ctx, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(config.Internal.Services).To(HaveLen(1))
				Expect(config.Internal.Services[0].Name).To(Equal("service1"))
				Expect(config.Internal.Services[0].FSMInstanceConfig.DesiredFSMState).To(Equal("running"))
				Expect(config.Internal.Redpanda.DesiredFSMState).To(Equal("running"))
				Expect(config.Agent.MetricsPort).To(Equal(8080))
				Expect(config.Agent.Location).To(HaveLen(2))
				Expect(config.Agent.Location[0]).To(Equal("Enterprise"))
				Expect(config.Agent.Location[1]).To(Equal("Site"))
			})

			It("should handle empty input", func() {
				config, err := ParseConfig([]byte{}, ctx, false)
				Expect(err).To(HaveOccurred())
				Expect(config).To(Equal(FullConfig{}))
			})

			It("should handle empty but valid YAML", func() {
				emptyYAML := "---\n"
				config, err := ParseConfig([]byte(emptyYAML), ctx, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(config).To(Equal(FullConfig{}))
			})

			It("should return error for malformed YAML", func() {
				malformedYAML := `
internal: {
  services: [
    { name: service1, desiredState: running,
`
				_, err := ParseConfig([]byte(malformedYAML), ctx, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("did not find expected node content"))
			})

			It("should return error for YAML with unknown fields when KnownFields is true", func() {
				yamlWithUnknownFields := `
internal:
  services:
    - name: service1
      desiredState: running
      unknownField: value
  unknownSection:
    key: value
`
				_, err := ParseConfig([]byte(yamlWithUnknownFields), ctx, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to decode config"))
			})

			It("should handle null values in YAML", func() {
				yamlWithNulls := `
internal:
  services:
    - name: service1
      desiredState: running
      s6ServiceConfig:
        command: null
        env: null
        configFiles: null
agent:
  location: null
`
				config, err := ParseConfig([]byte(yamlWithNulls), ctx, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.Internal.Services).To(HaveLen(1))
				Expect(config.Internal.Services[0].Name).To(Equal("service1"))
				Expect(config.Internal.Services[0].FSMInstanceConfig.DesiredFSMState).To(Equal("running"))
				Expect(config.Internal.Services[0].S6ServiceConfig.Command).To(BeNil())
				Expect(config.Internal.Services[0].S6ServiceConfig.Env).To(BeNil())
				Expect(config.Internal.Services[0].S6ServiceConfig.ConfigFiles).To(BeNil())
				Expect(config.Agent.Location).To(BeNil())
			})

			It("should handle nested complex structures", func() {
				complexYAML := `
internal:
  services:
    - name: service1
      desiredState: running
      s6ServiceConfig:
        command: ["/bin/sh", "-c", "echo 'complex command with spaces'"]
        env:
          COMPLEX_KEY: "value with spaces and \"quotes\""
          ANOTHER_KEY: 'single quoted value'
        configFiles:
          "file with spaces.txt": "content with multiple\nlines\nand \"quotes\""
`
				config, err := ParseConfig([]byte(complexYAML), ctx, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(config.Internal.Services).To(HaveLen(1))
				Expect(config.Internal.Services[0].S6ServiceConfig.Command).To(HaveLen(3))
				Expect(config.Internal.Services[0].S6ServiceConfig.Command[2]).To(Equal("echo 'complex command with spaces'"))
				Expect(config.Internal.Services[0].S6ServiceConfig.Env).To(HaveKeyWithValue("COMPLEX_KEY", "value with spaces and \"quotes\""))
				Expect(config.Internal.Services[0].S6ServiceConfig.Env).To(HaveKeyWithValue("ANOTHER_KEY", "single quoted value"))
				Expect(config.Internal.Services[0].S6ServiceConfig.ConfigFiles).To(HaveKeyWithValue("file with spaces.txt", "content with multiple\nlines\nand \"quotes\""))
			})
		})

		Context("with example YAML files from umh-core/examples", func() {
			var (
				// FileSystem service for reading example files
				fsService filesystem.Service
				ctx       context.Context
				cancel    context.CancelFunc
			)

			BeforeEach(func() {
				// Use the real filesystem for this test with a timeout context
				fsService = filesystem.NewDefaultService()
				ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
			})

			AfterEach(func() {
				cancel()
			})

			It("should parse all example .yaml files", func() {
				// List all files in the examples directory
				files, err := fsService.ReadDir(ctx, "../../examples")
				Expect(err).NotTo(HaveOccurred())

				for _, file := range files {
					// Skip non-YAML files
					if filepath.Ext(file.Name()) != ".yaml" {
						continue
					}

					By(fmt.Sprintf("Parsing %s", file.Name()))
					data, err := fsService.ReadFile(ctx, filepath.Join("../../examples", file.Name()))
					Expect(err).NotTo(HaveOccurred())

					_, err = ParseConfig(data, ctx, false)
					Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to parse %s", file.Name()))
				}
			})

			It("should extract templates from the templated protocol converter example", func() {
				// Test specifically with the example file that has templates
				data, err := fsService.ReadFile(ctx, "../../examples/example-config-protocolconverter-templated.yaml")
				Expect(err).NotTo(HaveOccurred())

				config, err := ParseConfig(data, ctx, true)
				Expect(err).NotTo(HaveOccurred())

				// The example should have at least one protocol converter using a template
				Expect(config.ProtocolConverter).NotTo(BeEmpty())
			})
		})
	})

	Describe("Round-trip config handling", func() {
		var (
			fsService filesystem.Service
			ctx       context.Context
			cancel    context.CancelFunc
		)

		BeforeEach(func() {
			fsService = filesystem.NewDefaultService()
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		})

		AfterEach(func() {
			cancel()
		})

		Context("with templated protocol converter example", func() {
			It("should read, parse, and write the config preserving templates", func() {
				// Read the original example file
				originalData, err := fsService.ReadFile(ctx, "../../examples/example-config-protocolconverter-templated.yaml")
				Expect(err).NotTo(HaveOccurred())

				// Parse the config with anchor extraction enabled
				config, err := ParseConfig(originalData, ctx, true)
				Expect(err).NotTo(HaveOccurred())

				// Verify we have the expected structure
				Expect(config.ProtocolConverter).To(HaveLen(3))
				Expect(config.Templates.ProtocolConverter).To(BeEmpty())

				// Find the temperature-sensor-pc that uses the template
				var tempSensorPC *ProtocolConverterConfig
				for _, pc := range config.ProtocolConverter {
					if pc.Name == "temperature-sensor-pc" {
						tempSensorPC = &pc
						break
					}
				}
				Expect(tempSensorPC).NotTo(BeNil())
				Expect(tempSensorPC.DesiredFSMState).To(Equal("active"))

				// Write the config using the config manager
				configManager.WithFileSystemService(mockFS)

				// Set up mock filesystem for writing
				var writtenData []byte
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})
				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data
					return nil
				})
				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})

				// Write the config
				err = configManager.writeConfig(ctx, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify it's still valid
				writtenConfig, err := ParseConfig(writtenData, ctx, true)
				Expect(err).NotTo(HaveOccurred())

				// Verify the structure is preserved
				Expect(writtenConfig.ProtocolConverter).To(HaveLen(3))
				Expect(writtenConfig.Agent.Location).To(HaveKeyWithValue(0, "plant-A"))
				Expect(writtenConfig.Agent.Location).To(HaveKeyWithValue(1, "line-4"))

				// Verify the protocol converters are preserved
				var writtenTempSensorPC *ProtocolConverterConfig
				for _, pc := range writtenConfig.ProtocolConverter {
					if pc.Name == "temperature-sensor-pc" {
						writtenTempSensorPC = &pc
						break
					}
				}
				Expect(writtenTempSensorPC).NotTo(BeNil())
				Expect(writtenTempSensorPC.DesiredFSMState).To(Equal("active"))
				Expect(writtenTempSensorPC.ProtocolConverterServiceConfig.Variables.User).To(HaveKeyWithValue("IP", "10.0.1.50"))
				Expect(writtenTempSensorPC.ProtocolConverterServiceConfig.Variables.User).To(HaveKeyWithValue("PORT", "4840"))
			})
		})

		Context("with templated stream processor example", func() {
			It("should read, parse, and write the config preserving data models, contracts, and templates", func() {
				// Read the original example file
				originalData, err := fsService.ReadFile(ctx, "../../examples/example-config-streamprocessor-templated.yaml")
				Expect(err).NotTo(HaveOccurred())

				// Parse the config with anchor extraction enabled
				config, err := ParseConfig(originalData, ctx, true)
				Expect(err).NotTo(HaveOccurred())

				// Verify we have the expected structure
				Expect(config.StreamProcessor).To(HaveLen(3))
				Expect(config.PayloadShapes).To(HaveLen(3))
				Expect(config.DataModels).To(HaveLen(2))
				Expect(config.DataContracts).To(HaveLen(2))
				Expect(config.Templates.StreamProcessor).To(BeEmpty())

				// Verify payload shapes
				Expect(config.PayloadShapes).To(HaveKey("timeseries-number"))
				Expect(config.PayloadShapes).To(HaveKey("timeseries-string"))
				Expect(config.PayloadShapes).To(HaveKey("vibration-data"))

				// Verify data models
				pumpModel := config.DataModels[0]
				if pumpModel.Name != "pump" {
					pumpModel = config.DataModels[1]
				}
				Expect(pumpModel.Name).To(Equal("pump"))
				Expect(pumpModel.Description).To(Equal("pump from vendor ABC"))
				Expect(pumpModel.Versions).To(HaveKey("v1"))

				// Verify data contracts
				Expect(config.DataContracts[0].Name).To(Equal("_pump-contract_v1"))
				Expect(config.DataContracts[0].Model.Name).To(Equal("pump"))
				Expect(config.DataContracts[0].Model.Version).To(Equal("v1"))
				Expect(config.DataContracts[1].Name).To(Equal("_raw"))
				Expect(config.DataContracts[1].Model).To(BeNil())
				Expect(config.DataContracts[1].DefaultBridges).To(BeEmpty())

				// Find the pump-processor-1 that uses the template
				var pumpProcessor1 *StreamProcessorConfig
				for _, sp := range config.StreamProcessor {
					if sp.Name == "pump-processor" {
						pumpProcessor1 = &sp
						break
					}
				}
				Expect(pumpProcessor1).NotTo(BeNil())
				Expect(pumpProcessor1.DesiredFSMState).To(Equal("active"))
				Expect(pumpProcessor1.StreamProcessorServiceConfig.TemplateRef).To(Equal("pump-processor"))

				// Write the config using the config manager
				configManager.WithFileSystemService(mockFS)

				// Set up mock filesystem for writing
				var writtenData []byte
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})
				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data
					return nil
				})
				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})

				// Write the config
				err = configManager.writeConfig(ctx, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify it's still valid
				writtenConfig, err := ParseConfig(writtenData, ctx, true)
				Expect(err).NotTo(HaveOccurred())

				// Verify the structure is preserved
				Expect(writtenConfig.StreamProcessor).To(HaveLen(3))
				Expect(writtenConfig.PayloadShapes).To(HaveLen(3))
				Expect(writtenConfig.DataModels).To(HaveLen(2))
				Expect(writtenConfig.DataContracts).To(HaveLen(2))
				Expect(writtenConfig.Agent.Location).To(HaveKeyWithValue(0, "factory-A"))
				Expect(writtenConfig.Agent.Location).To(HaveKeyWithValue(1, "line-1"))

				// Verify the stream processors are preserved
				var writtenPumpProcessor1 *StreamProcessorConfig
				for _, sp := range writtenConfig.StreamProcessor {
					if sp.Name == "pump-processor" {
						writtenPumpProcessor1 = &sp
						break
					}
				}
				Expect(writtenPumpProcessor1).NotTo(BeNil())
				Expect(writtenPumpProcessor1.DesiredFSMState).To(Equal("active"))
				Expect(writtenPumpProcessor1.StreamProcessorServiceConfig.Variables.User).To(HaveKeyWithValue("SERIAL_NUMBER", "ABC-12345"))
				Expect(writtenPumpProcessor1.StreamProcessorServiceConfig.Location).To(HaveKeyWithValue("2", "station-7"))
				Expect(writtenPumpProcessor1.StreamProcessorServiceConfig.Location).To(HaveKeyWithValue("3", "pump-001"))

				// Verify the inline config stream processor
				var directProcessor *StreamProcessorConfig
				for _, sp := range writtenConfig.StreamProcessor {
					if sp.Name == "motor-direct-processor" {
						directProcessor = &sp
						break
					}
				}
				Expect(directProcessor).NotTo(BeNil())
				Expect(directProcessor.StreamProcessorServiceConfig.Config.Model.Name).To(Equal("motor"))
				Expect(directProcessor.StreamProcessorServiceConfig.Variables.User).To(HaveKeyWithValue("STATUS", "operational"))
			})
		})
	})

	Describe("Background refresh with large config", func() {
		var (
			fsService filesystem.Service
			ctx       context.Context
			cancel    context.CancelFunc
		)

		BeforeEach(func() {
			fsService = filesystem.NewDefaultService()
			ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		})

		AfterEach(func() {
			// delete the test_cfg.yaml file
			err := fsService.Remove(ctx, "test_cfg.yaml")
			Expect(err).NotTo(HaveOccurred())
			cancel()
		})

		It("should update config via background refresh when file changes", func() {
			// Setup filesystem and config path
			configManager.WithFileSystemService(fsService)
			configManager.WithConfigPath("test_cfg.yaml")

			numGenerators := 5000
			// Read the original example file and write initial config
			originalData, err := fsService.ReadFile(ctx, "../../examples/example-config-protocolconverter-templated.yaml")
			Expect(err).NotTo(HaveOccurred())

			// Parse and write initial config
			config, err := ParseConfig(originalData, ctx, true)
			Expect(err).NotTo(HaveOccurred())

			err = configManager.writeConfig(ctx, config)
			Expect(err).NotTo(HaveOccurred())

			// Get initial config to populate cache and wait for background refresh
			Eventually(func() error {
				_, err := configManager.GetConfig(ctx, 0)
				return err
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			Eventually(func() error {
				cfg, err := configManager.GetConfig(ctx, 0)
				if err != nil {
					return err
				}
				if len(cfg.ProtocolConverter) != 3 {
					return fmt.Errorf("expected %d processors but got %d", 3, len(cfg.ProtocolConverter))
				}
				return nil
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			// Generate large config with numGenerators processors
			largeConfig, err := GenerateConfig(numGenerators, configManager)
			Expect(err).NotTo(HaveOccurred())

			// Write the large config to trigger background refresh
			err = configManager.writeConfig(ctx, largeConfig)
			Expect(err).NotTo(HaveOccurred())

			// ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
			// _, err = configManager.GetConfig(ctx2, 0)
			// Expect(err).To(HaveOccurred())
			// cancel2()

			// Poll GetConfig until background refresh picks up the changes
			start := time.Now()
			var finalConfig FullConfig
			const maxWaitTime = 10 * time.Second

			Eventually(func() error {
				// Check if we've exceeded max wait time
				if time.Since(start) > maxWaitTime {
					return fmt.Errorf("exceeded max wait time")
				}

				var err error
				newCtx, cancel := context.WithTimeout(context.Background(), constants.ConfigGetConfigTimeout)
				finalConfig, err = configManager.GetConfig(newCtx, 0)
				cancel()
				if err != nil {
					return fmt.Errorf("failed to get config: %w", err)
				}

				// Count processors in the updated config
				processorCount := 0
				if len(finalConfig.ProtocolConverter) > 0 {
					pipeline := finalConfig.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline
					if processors, ok := pipeline["processors"].(map[string]any); ok {
						processorCount = len(processors)
					}
				}

				// Do the comparison inside the function
				if processorCount != numGenerators {
					return fmt.Errorf("expected %d processors but got %d", numGenerators, processorCount)
				}

				return nil
			}, "10s", "100ms").Should(Succeed(), "Background refresh should update config with %d processors", numGenerators)

			// Verify the final config actually contains the expected processors
			Expect(len(finalConfig.ProtocolConverter)).To(BeNumerically(">=", 1))
			pipeline := finalConfig.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Pipeline
			processors, ok := pipeline["processors"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(len(processors)).To(Equal(numGenerators))

			// Measure final GetConfig performance
			perfStart := time.Now()
			_, err = configManager.GetConfig(ctx, 0)
			Expect(err).NotTo(HaveOccurred())
			duration := time.Since(perfStart)

			fmt.Printf("GetConfig duration with %d processors: %v\n", numGenerators, duration)
			Expect(duration).To(BeNumerically("<", constants.ConfigBackgroundRefreshTimeout))
		})
	})
})
