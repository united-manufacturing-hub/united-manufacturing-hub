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
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("LoadConfigWithEnvOverrides", func() {
	var (
		mockFS        *filesystem.MockFileSystem
		configManager *FileConfigManager
		ctx           context.Context
		log           *zap.SugaredLogger
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		ctx = context.Background()
		log = logger.For(logger.ComponentConfigManager)
	})

	JustBeforeEach(func() {
		configManager = NewFileConfigManager()
		configManager.WithFileSystemService(mockFS)
	})

	Context("when ENABLE_RESOURCE_LIMIT_BLOCKING environment variable is set to false", func() {
		validYAML := `
agent:
  metricsPort: 8080
  enableResourceLimitBlocking: true
`
		BeforeEach(func() {
			Expect(os.Setenv("ENABLE_RESOURCE_LIMIT_BLOCKING", "false")).To(Succeed())

			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				return []byte(validYAML), nil
			})

			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				return mockFS.NewMockFileInfo(
					DefaultConfigPath,
					int64(len(validYAML)),
					0644,
					time.Now(),
					false,
				), nil
			})

			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return nil
			})
		})

		AfterEach(func() {
			Expect(os.Unsetenv("ENABLE_RESOURCE_LIMIT_BLOCKING")).To(Succeed())
		})

		It("should override config to false", func() {
			Eventually(func() error {
				wrappedManager := &FileConfigManagerWithBackoff{
					configManager: configManager,
				}
				config, err := LoadConfigWithEnvOverrides(ctx, wrappedManager, log)
				if err != nil {
					return err
				}
				if config.Agent.EnableResourceLimitBlocking {
					return errors.New("expected EnableResourceLimitBlocking to be false, got true")
				}

				return nil
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())
		})
	})

	Context("when ENABLE_RESOURCE_LIMIT_BLOCKING environment variable is set to true", func() {
		validYAML := `
agent:
  metricsPort: 8080
  enableResourceLimitBlocking: false
`
		BeforeEach(func() {
			Expect(os.Setenv("ENABLE_RESOURCE_LIMIT_BLOCKING", "true")).To(Succeed())

			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				return []byte(validYAML), nil
			})

			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				return mockFS.NewMockFileInfo(
					DefaultConfigPath,
					int64(len(validYAML)),
					0644,
					time.Now(),
					false,
				), nil
			})

			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return nil
			})
		})

		AfterEach(func() {
			Expect(os.Unsetenv("ENABLE_RESOURCE_LIMIT_BLOCKING")).To(Succeed())
		})

		It("should override config to true", func() {
			Eventually(func() error {
				wrappedManager := &FileConfigManagerWithBackoff{
					configManager: configManager,
				}
				config, err := LoadConfigWithEnvOverrides(ctx, wrappedManager, log)
				if err != nil {
					return err
				}
				if !config.Agent.EnableResourceLimitBlocking {
					return errors.New("expected EnableResourceLimitBlocking to be true, got false")
				}

				return nil
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())
		})
	})

})
