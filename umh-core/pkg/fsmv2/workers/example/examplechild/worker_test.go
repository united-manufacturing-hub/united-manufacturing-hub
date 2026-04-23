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

package example_child_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
)

var _ = Describe("ChildWorker", func() {
	var (
		logger       deps.FSMLogger
		mockPool     *MockConnectionPool
		identity     deps.Identity
		dependencies *example_child.ExamplechildDependencies
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		mockPool = NewMockConnectionPool()
		identity = deps.Identity{ID: "test-id", Name: "test-child", WorkerType: example_child.WorkerTypeName}
		dependencies = example_child.NewExamplechildDependencies(mockPool, logger, nil, identity)
	})

	Describe("NewChildWorker", func() {
		It("should create a worker successfully", func() {
			worker, err := example_child.NewChildWorker(identity, logger, nil, dependencies)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should have a non-nil initial state", func() {
			worker, err := example_child.NewChildWorker(identity, logger, nil, dependencies)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker.GetInitialState()).NotTo(BeNil())
		})

		It("should provision default dependencies when nil is passed", func() {
			worker, err := example_child.NewChildWorker(identity, logger, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should reject nil logger", func() {
			_, err := example_child.NewChildWorker(identity, nil, nil, dependencies)

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return a valid observed state", func() {
			worker, err := example_child.NewChildWorker(identity, logger, nil, dependencies)
			Expect(err).NotTo(HaveOccurred())

			observed, err := worker.CollectObservedState(context.Background(), nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())
		})
	})
})
