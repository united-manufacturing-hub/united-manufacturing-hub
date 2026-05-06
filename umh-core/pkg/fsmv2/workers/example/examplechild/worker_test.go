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

package examplechild_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	examplechild "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
)

var _ = Describe("ChildWorker", func() {
	var (
		logger   deps.FSMLogger
		mockPool *MockConnectionPool
		identity deps.Identity
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		mockPool = NewMockConnectionPool()
		identity = deps.Identity{ID: "test-id", Name: "test-child"}
	})

	Describe("NewChildWorker", func() {
		It("should create a worker successfully", func() {
			worker, err := examplechild.NewChildWorker(identity, mockPool, logger, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should have a non-nil initial state", func() {
			worker, err := examplechild.NewChildWorker(identity, mockPool, logger, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(worker.GetInitialState()).NotTo(BeNil())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return a valid observed state", func() {
			worker, err := examplechild.NewChildWorker(identity, mockPool, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			observed, err := worker.CollectObservedState(context.Background(), nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			obs, ok := observed.(fsmv2.Observation[examplechild.ExamplechildStatus])
			Expect(ok).To(BeTrue(), "expected fsmv2.Observation[ExamplechildStatus], got %T", observed)
			Expect(obs.Status.ConnectionHealth).To(Equal("no connection"))
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should return running state for empty config", func() {
			worker, err := examplechild.NewChildWorker(identity, mockPool, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			spec := config.UserSpec{
				Config:    "",
				Variables: config.VariableBundle{},
			}

			desiredIface, err := worker.DeriveDesiredState(spec)

			Expect(err).NotTo(HaveOccurred())

			desired, ok := desiredIface.(*fsmv2.WrappedDesiredState[examplechild.ExamplechildConfig])
			Expect(ok).To(BeTrue(), "expected *WrappedDesiredState[ExamplechildConfig], got %T", desiredIface)
			Expect(desired.IsBeingRemoved()).To(BeFalse())
		})
	})

	Describe("GetDependenciesAny", func() {
		It("returns *ExamplechildDependencies", func() {
			worker, err := examplechild.NewChildWorker(identity, mockPool, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			var w fsmv2.Worker = worker
			dp, ok := w.(fsmv2.DependencyProvider)
			Expect(ok).To(BeTrue(), "worker must implement DependencyProvider")
			got := dp.GetDependenciesAny()
			_, ok = got.(*examplechild.ExamplechildDependencies)
			Expect(ok).To(BeTrue(), "expected *ExamplechildDependencies, got %T", got)
		})
	})
})
