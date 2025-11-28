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

package helpers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

var _ = Describe("BaseWorker", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewBaseWorker", func() {
		It("should create a non-nil worker", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			registry := fsmv2.NewBaseDependencies(logger, identity)
			worker := helpers.NewBaseWorker(registry)

			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("GetDependencies", func() {
		It("should return the registry passed to constructor", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			registry := fsmv2.NewBaseDependencies(logger, identity)
			worker := helpers.NewBaseWorker(registry)

			returnedRegistry := worker.GetDependencies()

			Expect(returnedRegistry).To(Equal(registry))
		})
	})

	Describe("Generic type parameter", func() {
		It("should work with concrete BaseDependencies type", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			registry := fsmv2.NewBaseDependencies(logger, identity)
			worker := helpers.NewBaseWorker[*fsmv2.BaseDependencies](registry)

			Expect(worker).NotTo(BeNil())
			Expect(worker.GetDependencies()).To(Equal(registry))
		})

		It("should work with any type implementing Dependencies interface", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := fsmv2.NewBaseDependencies(logger, identity)
			worker := helpers.NewBaseWorker[fsmv2.Dependencies](dependencies)

			Expect(worker).NotTo(BeNil())
			Expect(worker.GetDependencies()).To(Equal(dependencies))
			// Logger is enriched with worker context, so it won't equal the original
			Expect(worker.GetDependencies().GetLogger()).NotTo(BeNil())
		})
	})

	Describe("Embedding pattern", func() {
		type TestWorker struct {
			*helpers.BaseWorker[*fsmv2.BaseDependencies]
			customField string
		}

		It("should allow worker structs to embed BaseWorker", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			registry := fsmv2.NewBaseDependencies(logger, identity)
			testWorker := &TestWorker{
				BaseWorker:  helpers.NewBaseWorker(registry),
				customField: "test-value",
			}

			Expect(testWorker.GetDependencies()).To(Equal(registry))
			Expect(testWorker.customField).To(Equal("test-value"))
		})

		It("should provide direct access to registry through embedded BaseWorker", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			registry := fsmv2.NewBaseDependencies(logger, identity)
			testWorker := &TestWorker{
				BaseWorker:  helpers.NewBaseWorker(registry),
				customField: "test-value",
			}

			// Logger is enriched with worker context
			Expect(testWorker.GetDependencies().GetLogger()).NotTo(BeNil())
		})
	})
})
