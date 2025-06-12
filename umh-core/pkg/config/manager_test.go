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
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

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
				config, err := configManager.GetConfig(ctx, tick)
				Expect(err).NotTo(HaveOccurred())

				Expect(config.Internal.Services).To(HaveLen(1))
				Expect(config.Internal.Services[0].Name).To(Equal("service1"))
				Expect(config.Internal.Services[0].FSMInstanceConfig.DesiredFSMState).To(Equal("running"))
				Expect(config.Internal.Services[0].S6ServiceConfig.Command).To(Equal([]string{"/bin/echo", "hello world"}))
				Expect(config.Internal.Services[0].S6ServiceConfig.Env).To(HaveKeyWithValue("KEY", "value"))
				Expect(config.Internal.Services[0].S6ServiceConfig.ConfigFiles).To(HaveKeyWithValue("file.txt", "content"))
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
				_, err := configManager.GetConfig(ctx, tick)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse config file"))
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
				_, err := configManager.GetConfig(ctx, tick)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read config file"))
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
				config, _, err := parseConfig([]byte(validYAML), false)
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

			//TODO: fix this test cases
			// It("should handle empty input", func() {
			// 	config, _, err := parseConfig([]byte{}, false)
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(config).To(Equal(FullConfig{}))
			// })

			// It("should handle empty but valid YAML", func() {
			// 	emptyYAML := "---\n"
			// 	config, _, err := parseConfig([]byte(emptyYAML), false)
			// 	Expect(err).ToNot(HaveOccurred())
			// 	Expect(config).To(Equal(FullConfig{}))
			// })

			It("should return error for malformed YAML", func() {
				malformedYAML := `
internal: {
  services: [
    { name: service1, desiredState: running,
`
				_, _, err := parseConfig([]byte(malformedYAML), false)
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
				_, _, err := parseConfig([]byte(yamlWithUnknownFields), false)
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
				config, _, err := parseConfig([]byte(yamlWithNulls), false)
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
				config, _, err := parseConfig([]byte(complexYAML), false)
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

					_, _, err = parseConfig(data, false)
					Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to parse %s", file.Name()))
				}
			})

			It("should extract anchors from the templated protocol converter example", func() {
				// Test specifically with the example file that has anchors
				data, err := fsService.ReadFile(ctx, "../../examples/example-config-protocolconverter-templated.yaml")
				Expect(err).NotTo(HaveOccurred())

				config, anchorMap, err := parseConfig(data, true)
				Expect(err).NotTo(HaveOccurred())

				// The example should have at least one protocol converter using an anchor
				Expect(config.ProtocolConverter).NotTo(BeEmpty())

				// Check that we extracted anchor mappings for protocol converters that use aliases
				// The temperature-sensor-pc should use the opcua_http anchor
				if len(anchorMap) > 0 {
					By("Verifying extracted anchor mappings")
					for pcName, anchorName := range anchorMap {
						Expect(pcName).NotTo(BeEmpty(), "Protocol converter name should not be empty")
						Expect(anchorName).NotTo(BeEmpty(), "Anchor name should not be empty")

						// Find the corresponding protocol converter
						var foundPC *ProtocolConverterConfig
						for _, pc := range config.ProtocolConverter {
							if pc.Name == pcName {
								foundPC = &pc
								break
							}
						}
						Expect(foundPC).NotTo(BeNil(), fmt.Sprintf("Should find protocol converter %s", pcName))

						// The template should be empty since it was replaced
						Expect(foundPC.ProtocolConverterServiceConfig.Template.ConnectionServiceConfig.NmapTemplate).To(BeNil(),
							fmt.Sprintf("Template should be empty for PC %s using anchor %s", pcName, anchorName))
					}
				}
			})
		})
	})
})
