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

package example_parent_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	example_parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
)

var _ = Describe("ParentDependencies", func() {
	var (
		logger       *zap.SugaredLogger
		configLoader *MockConfigLoader
		deps         *example_parent.ParentDependencies
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		configLoader = NewMockConfigLoader()
	})

	Describe("NewParentDependencies", func() {
		It("should create dependencies with valid inputs", func() {
			deps = example_parent.NewParentDependencies(configLoader, logger)

			Expect(deps).NotTo(BeNil())
			Expect(deps.GetConfigLoader()).To(Equal(configLoader))
			Expect(deps.GetLogger()).To(Equal(logger))
		})
	})

	Describe("GetConfigLoader", func() {
		BeforeEach(func() {
			deps = example_parent.NewParentDependencies(configLoader, logger)
		})

		It("should return the config loader", func() {
			result := deps.GetConfigLoader()
			Expect(result).To(Equal(configLoader))
		})
	})
})

// MockConfigLoader implements ConfigLoader interface for testing
type MockConfigLoader struct {
	LoadConfigResult  map[string]interface{}
	LoadConfigError   error
	LoadConfigCalled  bool
}

func NewMockConfigLoader() *MockConfigLoader {
	return &MockConfigLoader{
		LoadConfigResult: make(map[string]interface{}),
	}
}

func (m *MockConfigLoader) LoadConfig() (map[string]interface{}, error) {
	m.LoadConfigCalled = true
	return m.LoadConfigResult, m.LoadConfigError
}
