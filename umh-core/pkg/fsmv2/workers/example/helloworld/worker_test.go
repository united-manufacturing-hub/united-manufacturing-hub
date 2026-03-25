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

package hello_world_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
)

var _ = Describe("HelloworldWorker", func() {
	var (
		worker fsmv2.Worker
		logger deps.FSMLogger
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
	})

	Describe("NewHelloworldWorker", func() {
		It("should create worker successfully", func() {
			identity := deps.Identity{ID: "test-worker"}
			w, err := hello_world.NewHelloworldWorker(identity, logger, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(w).NotTo(BeNil())
		})

		It("should fail with nil logger", func() {
			identity := deps.Identity{ID: "test-worker"}
			w, err := hello_world.NewHelloworldWorker(identity, nil, nil)

			Expect(err).To(HaveOccurred())
			Expect(w).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("logger must not be nil"))
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			identity := deps.Identity{ID: "test-worker"}
			var err error
			worker, err = hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should collect initial state with HelloSaid=false", func() {
			desired := &fsmv2.WrappedDesiredState[hello_world.HelloworldConfig]{}
			obs, err := worker.CollectObservedState(context.Background(), desired)

			Expect(err).NotTo(HaveOccurred())
			typedObs, ok := obs.(fsmv2.WrappedObservedState[hello_world.HelloworldStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HelloSaid).To(BeFalse())
		})

		It("should return error on cancelled context", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			desired := &fsmv2.WrappedDesiredState[hello_world.HelloworldConfig]{}
			obs, err := worker.CollectObservedState(ctx, desired)

			Expect(err).To(Equal(context.Canceled))
			Expect(obs).To(BeNil())
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			identity := deps.Identity{ID: "test-worker"}
			var err error
			worker, err = hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return Stopped state", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).NotTo(BeNil())
			Expect(initialState.String()).To(Equal("Stopped"))
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			identity := deps.Identity{ID: "test-worker"}
			var err error
			worker, err = hello_world.NewHelloworldWorker(identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return running state when spec is nil", func() {
			desired, err := worker.DeriveDesiredState(nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(desired).NotTo(BeNil())
		})
	})
})
