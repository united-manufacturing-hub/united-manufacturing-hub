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

package s6

import (
	"context"
	"os"
	"path/filepath"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("S6 Environment Variables File Creation", func() {
	var (
		ctx         context.Context
		service     *DefaultService
		mockFS      *filesystem.MockFileSystem
		servicePath string
		config      s6serviceconfig.S6ServiceConfig
		filesCreated map[string][]byte // Track what files were created
	)

	BeforeEach(func() {
		ctx = context.Background()
		service = &DefaultService{logger: logger.For("test")}
		mockFS = filesystem.NewMockFileSystem()
		filesCreated = make(map[string][]byte)

		servicePath = filepath.Join(constants.S6BaseDir, "test-env-service")

		// Base config with command
		config = s6serviceconfig.S6ServiceConfig{
			Command: []string{"/bin/sleep", "infinity"},
		}

		// Mock filesystem operations to track file creation
		mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
			filesCreated[path] = nil // Mark directory as created

			return nil
		})
		mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
			filesCreated[path] = data

			return nil
		})
		mockFS.WithSymlinkFunc(func(ctx context.Context, oldPath, newPath string) error {
			return nil
		})
		mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
			_, exists := filesCreated[path]

			return exists, nil
		})
		mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
			data, exists := filesCreated[path]

			return exists && data != nil, nil
		})
		mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
			if data, exists := filesCreated[path]; exists {
				return data, nil
			}

			return nil, os.ErrNotExist
		})
	})

	Describe("when Env map has entries", func() {
		BeforeEach(func() {
			config.Env = map[string]string{
				"OPC_DEBUG": "debug",
			}
		})

		It("should create env/ directory", func() {
			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			// Check that env/ directory exists in repository
			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")
			envDir := filepath.Join(repositoryDir, "env")

			exists, err := mockFS.PathExists(ctx, envDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "env/ directory should exist when Env map has entries")
		})

		It("should create environment variable file with correct content", func() {
			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			// Check that env/OPC_DEBUG file exists and contains "debug"
			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")
			envFile := filepath.Join(repositoryDir, "env", "OPC_DEBUG")

			exists, err := mockFS.FileExists(ctx, envFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "env/OPC_DEBUG file should exist")

			content, err := mockFS.ReadFile(ctx, envFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("debug"), "env/OPC_DEBUG file should contain 'debug'")
		})
	})

	Describe("when Env map has multiple entries", func() {
		BeforeEach(func() {
			config.Env = map[string]string{
				"OPC_DEBUG":   "debug",
				"OPC_TIMEOUT": "5000",
				"OPC_RETRIES": "3",
			}
		})

		It("should create multiple environment variable files", func() {
			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")

			// Check all three files exist with correct content
			expectedFiles := map[string]string{
				"OPC_DEBUG":   "debug",
				"OPC_TIMEOUT": "5000",
				"OPC_RETRIES": "3",
			}

			for varName, expectedValue := range expectedFiles {
				envFile := filepath.Join(repositoryDir, "env", varName)

				exists, err := mockFS.FileExists(ctx, envFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue(), "env/%s file should exist", varName)

				content, err := mockFS.ReadFile(ctx, envFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(expectedValue), "env/%s should contain '%s'", varName, expectedValue)
			}
		})
	})

	Describe("when Env map is empty", func() {
		BeforeEach(func() {
			config.Env = map[string]string{}
		})

		It("should not create env/ directory", func() {
			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")
			envDir := filepath.Join(repositoryDir, "env")

			exists, err := mockFS.PathExists(ctx, envDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "env/ directory should not exist when Env map is empty")
		})
	})

	Describe("when Env map is nil", func() {
		BeforeEach(func() {
			config.Env = nil
		})

		It("should not create env/ directory", func() {
			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")
			envDir := filepath.Join(repositoryDir, "env")

			exists, err := mockFS.PathExists(ctx, envDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "env/ directory should not exist when Env map is nil")
		})
	})

	Describe("security validation", func() {
		It("should reject empty variable names", func() {
			config.Env = map[string]string{"": "value"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty"))
		})

		It("should reject path traversal with ..", func() {
			config.Env = map[string]string{"../../../etc/passwd": "value"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid environment variable name"))
		})

		It("should reject path traversal with /", func() {
			config.Env = map[string]string{"dir/file": "value"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid environment variable name"))
		})

		It("should reject path traversal with \\", func() {
			config.Env = map[string]string{"dir\\file": "value"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid environment variable name"))
		})

		It("should reject null bytes in variable names", func() {
			config.Env = map[string]string{"VAR\x00NAME": "value"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid characters"))
		})

		It("should reject newlines in variable names", func() {
			config.Env = map[string]string{"VAR\nNAME": "value"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid characters"))
		})

		It("should reject carriage returns in variable names", func() {
			config.Env = map[string]string{"VAR\rNAME": "value"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid characters"))
		})
	})

	Describe("edge cases", func() {
		It("should handle large values", func() {
			bytes := make([]byte, 10000)
			for i := range bytes {
				bytes[i] = 'A'
			}
			largeValue := string(bytes)
			config.Env = map[string]string{"LARGE_VAR": largeValue}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")
			envFile := filepath.Join(repositoryDir, "env", "LARGE_VAR")

			content, err := mockFS.ReadFile(ctx, envFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(content)).To(Equal(10000))
		})

		It("should handle newlines in values", func() {
			config.Env = map[string]string{"MULTILINE": "line1\nline2\nline3"}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")
			envFile := filepath.Join(repositoryDir, "env", "MULTILINE")

			content, err := mockFS.ReadFile(ctx, envFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("line1\nline2\nline3"))
		})

		It("should handle special characters in values", func() {
			config.Env = map[string]string{
				"QUOTES": `value with "quotes"`,
				"TABS":   "value\twith\ttabs",
			}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")

			quotesFile := filepath.Join(repositoryDir, "env", "QUOTES")
			quotesContent, err := mockFS.ReadFile(ctx, quotesFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(quotesContent)).To(Equal(`value with "quotes"`))

			tabsFile := filepath.Join(repositoryDir, "env", "TABS")
			tabsContent, err := mockFS.ReadFile(ctx, tabsFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(tabsContent)).To(Equal("value\twith\ttabs"))
		})

		It("should handle valid unusual variable names", func() {
			config.Env = map[string]string{
				"VAR_123":   "value1",
				"VAR-NAME":  "value2",
				"lowercase": "value3",
			}

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")

			expectedFiles := map[string]string{
				"VAR_123":   "value1",
				"VAR-NAME":  "value2",
				"lowercase": "value3",
			}

			for varName, expectedValue := range expectedFiles {
				envFile := filepath.Join(repositoryDir, "env", varName)

				exists, err := mockFS.FileExists(ctx, envFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue(), "env/%s file should exist", varName)

				content, err := mockFS.ReadFile(ctx, envFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(Equal(expectedValue), "env/%s should contain '%s'", varName, expectedValue)
			}
		})
	})

	Describe("round-trip integration", func() {
		It("should preserve environment variables through create and read", func() {
			originalEnv := map[string]string{
				"OPC_DEBUG": "debug",
				"OPC_PORT":  "4840",
				"CUSTOM":    "value",
			}
			config.Env = originalEnv

			err := service.Create(ctx, servicePath, config, mockFS)
			Expect(err).NotTo(HaveOccurred())

			repositoryDir := filepath.Join(constants.GetS6RepositoryBaseDir(), "test-env-service")
			envDir := filepath.Join(repositoryDir, "env")

			retrievedEnv := make(map[string]string)
			for varName := range originalEnv {
				envFile := filepath.Join(envDir, varName)
				content, err := mockFS.ReadFile(ctx, envFile)
				Expect(err).NotTo(HaveOccurred())
				retrievedEnv[varName] = string(content)
			}

			Expect(retrievedEnv).To(Equal(originalEnv))
		})
	})
})
