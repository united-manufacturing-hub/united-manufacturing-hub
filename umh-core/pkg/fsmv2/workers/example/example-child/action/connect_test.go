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

	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/action"
)

var _ = Describe("ConnectAction", func() {
	var (
		ctx     context.Context
		depsAny any
	)

	BeforeEach(func() {
		ctx = context.Background()
		depsAny = example_child.NewChildDependencies(&example_child.DefaultConnectionPool{}, zap.NewNop().Sugar(), "child", "test-id")
	})

	Describe("Execute", func() {
		Context("when executing successfully", func() {
			It("should complete without error", func() {
				connectAction := &action.ConnectAction{}

				err := connectAction.Execute(ctx, depsAny)

				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when created with failures", func() {
			It("should succeed on all executions (skeleton implementation)", func() {
				connectAction := action.NewConnectActionWithFailures(2)

				err1 := connectAction.Execute(ctx, depsAny)
				Expect(err1).ToNot(HaveOccurred(), "first execution should succeed (skeleton implementation)")

				err2 := connectAction.Execute(ctx, depsAny)
				Expect(err2).ToNot(HaveOccurred(), "second execution should succeed (skeleton implementation)")

				err3 := connectAction.Execute(ctx, depsAny)
				Expect(err3).ToNot(HaveOccurred(), "third execution should succeed (skeleton implementation)")
			})
		})
	})

	Describe("Name", func() {
		It("should return the correct action name", func() {
			connectAction := &action.ConnectAction{}

			Expect(connectAction.Name()).To(Equal(action.ConnectActionName))
		})
	})
})
