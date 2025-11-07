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


package fsmv2_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

func TestFsmv2Dependencies(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FSMv2 Dependencies Suite")
}

var _ = Describe("BaseDependencies", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewBaseDependencies", func() {
		It("should create a non-nil dependencies", func() {
			dependencies := fsmv2.NewBaseDependencies(logger)
			Expect(dependencies).NotTo(BeNil())
		})

		It("should return the logger passed to constructor", func() {
			dependencies := fsmv2.NewBaseDependencies(logger)
			Expect(dependencies.GetLogger()).To(Equal(logger))
		})

		It("should panic when logger is nil", func() {
			Expect(func() {
				fsmv2.NewBaseDependencies(nil)
			}).To(Panic())
		})
	})

	Describe("Dependencies interface compliance", func() {
		It("should implement Dependencies interface", func() {
			dependencies := fsmv2.NewBaseDependencies(logger)
			var _ fsmv2.Dependencies = dependencies
		})
	})
})
