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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var _ = Describe("BaseWorker", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewBaseWorker", func() {
		It("should create a non-nil worker", func() {
			registry := fsmv2.NewBaseDependencies(logger)
			worker := fsmv2.NewBaseWorker(registry)

			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("GetDependencies", func() {
		It("should return the registry passed to constructor", func() {
			registry := fsmv2.NewBaseDependencies(logger)
			worker := fsmv2.NewBaseWorker(registry)

			returnedRegistry := worker.GetDependencies()

			Expect(returnedRegistry).To(Equal(registry))
		})
	})

	Describe("Generic type parameter", func() {
		It("should work with concrete BaseDependencies type", func() {
			registry := fsmv2.NewBaseDependencies(logger)
			worker := fsmv2.NewBaseWorker[*fsmv2.BaseDependencies](registry)

			Expect(worker).NotTo(BeNil())
			Expect(worker.GetDependencies()).To(Equal(registry))
		})

		It("should work with any type implementing Dependencies interface", func() {
			dependencies := fsmv2.NewBaseDependencies(logger)
			worker := fsmv2.NewBaseWorker[fsmv2.Dependencies](dependencies)

			Expect(worker).NotTo(BeNil())
			Expect(worker.GetDependencies()).To(Equal(dependencies))
			Expect(worker.GetDependencies().GetLogger()).To(Equal(logger))
		})
	})

	Describe("Embedding pattern", func() {
		type TestWorker struct {
			*fsmv2.BaseWorker[*fsmv2.BaseDependencies]
			customField string
		}

		It("should allow worker structs to embed BaseWorker", func() {
			registry := fsmv2.NewBaseDependencies(logger)
			testWorker := &TestWorker{
				BaseWorker:  fsmv2.NewBaseWorker(registry),
				customField: "test-value",
			}

			Expect(testWorker.GetDependencies()).To(Equal(registry))
			Expect(testWorker.customField).To(Equal("test-value"))
		})

		It("should provide direct access to registry through embedded BaseWorker", func() {
			registry := fsmv2.NewBaseDependencies(logger)
			testWorker := &TestWorker{
				BaseWorker:  fsmv2.NewBaseWorker(registry),
				customField: "test-value",
			}

			Expect(testWorker.GetDependencies().GetLogger()).To(Equal(logger))
		})
	})
})
