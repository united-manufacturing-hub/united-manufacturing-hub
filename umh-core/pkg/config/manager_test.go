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
				Expect(err.Error()).To(Equal("file check failed"))
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
})
