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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

var _ = Describe("HelloworldWorker", func() {
	var (
		worker *hello_world.HelloworldWorker
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()
	})

	Describe("NewHelloworldWorker", func() {
		It("should create worker with derived worker type", func() {
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
			obs, err := worker.CollectObservedState(context.Background())

			Expect(err).NotTo(HaveOccurred())
			typedObs, ok := obs.(snapshot.HelloworldObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.HelloSaid).To(BeFalse())
		})

		It("should return error on cancelled context", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			obs, err := worker.CollectObservedState(ctx)

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

		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()

			_, ok := initialState.(*state.StoppedState)
			Expect(ok).To(BeTrue())
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
