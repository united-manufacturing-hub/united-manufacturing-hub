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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
)

var _ = Describe("ChildWorker", func() {
	var (
		logger   *zap.SugaredLogger
		mockPool *MockConnectionPool
		identity deps.Identity
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockPool = NewMockConnectionPool()
		identity = deps.Identity{ID: "test-id", Name: "test-child"}
	})

	Describe("NewChildWorker", func() {
		It("should create a worker successfully", func() {
			worker, err := example_child.NewChildWorker(identity, mockPool, logger, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should have a non-nil initial state", func() {
			worker, err := example_child.NewChildWorker(identity, mockPool, logger, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker.GetInitialState()).NotTo(BeNil())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return a valid observed state", func() {
			worker, err := example_child.NewChildWorker(identity, mockPool, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			observed, err := worker.CollectObservedState(context.Background())

			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should return running state for empty config", func() {
			worker, err := example_child.NewChildWorker(identity, mockPool, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			spec := config.UserSpec{
				Config:    "",
				Variables: config.VariableBundle{},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)

			Expect(err).NotTo(HaveOccurred())

			desired := desiredIface.(*config.DesiredState)
			Expect(desired.State).To(Equal(config.DesiredStateRunning))
		})
	})
})
