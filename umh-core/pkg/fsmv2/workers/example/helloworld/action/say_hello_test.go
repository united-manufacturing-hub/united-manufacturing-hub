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

package action_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/action"
)

var _ = Describe("SayHelloAction", func() {
	var (
		act          *action.SayHelloAction
		dependencies *hello_world.HelloworldDependencies
		logger       *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		identity := deps.Identity{ID: "test-id", WorkerType: "helloworld"}
		dependencies = hello_world.NewHelloworldDependencies(logger, nil, identity)
		act = &action.SayHelloAction{}
	})

	Describe("Name", func() {
		It("should return action name", func() {
			Expect(act.Name()).To(Equal("say_hello"))
		})
	})

	Describe("String", func() {
		It("should return action name", func() {
			Expect(act.String()).To(Equal("say_hello"))
		})
	})

	Describe("Execute", func() {
		Context("when HelloSaid is false", func() {
			It("should set HelloSaid to true", func() {
				Expect(dependencies.HasSaidHello()).To(BeFalse())

				err := act.Execute(context.Background(), dependencies)

				Expect(err).NotTo(HaveOccurred())
				Expect(dependencies.HasSaidHello()).To(BeTrue())
			})
		})

		Context("when HelloSaid is already true (idempotency)", func() {
			BeforeEach(func() {
				dependencies.SetHelloSaid(true)
			})

			It("should remain true without error", func() {
				Expect(dependencies.HasSaidHello()).To(BeTrue())

				err := act.Execute(context.Background(), dependencies)

				Expect(err).NotTo(HaveOccurred())
				Expect(dependencies.HasSaidHello()).To(BeTrue())
			})
		})

		Context("context cancellation", func() {
			It("should return error on cancelled context", func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				err := act.Execute(ctx, dependencies)

				Expect(err).To(Equal(context.Canceled))
			})
		})
	})

	Describe("Idempotency (Invariant I10)", func() {
		It("should be idempotent when called multiple times", func() {
			ctx := context.Background()

			// First call
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.HasSaidHello()).To(BeTrue())

			// Second call (should be no-op)
			err = act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.HasSaidHello()).To(BeTrue())

			// Third call (should be no-op)
			err = act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.HasSaidHello()).To(BeTrue())
		})
	})
})
