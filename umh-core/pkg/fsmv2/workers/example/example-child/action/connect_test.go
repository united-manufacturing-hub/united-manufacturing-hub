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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/action"
)

type mockDeps struct {
	*fsmv2.BaseDependencies
}

func newMockDeps() *mockDeps {
	return &mockDeps{
		BaseDependencies: fsmv2.NewBaseDependencies(zap.NewNop().Sugar()),
	}
}

var _ = Describe("ConnectAction", func() {
	var (
		ctx     context.Context
		depsAny any
	)

	BeforeEach(func() {
		ctx = context.Background()
		depsAny = newMockDeps()
	})

	Describe("Execute", func() {
		Context("when executing successfully", func() {
			It("should complete without error", func() {
				connectAction := action.NewConnectAction()

				err := connectAction.Execute(ctx, depsAny)

				Expect(err).To(BeNil())
			})
		})

		Context("when created with failures", func() {
			It("should succeed on all executions (skeleton implementation)", func() {
				connectAction := action.NewConnectActionWithFailures(2)

				err1 := connectAction.Execute(ctx, depsAny)
				Expect(err1).To(BeNil(), "first execution should succeed (skeleton implementation)")

				err2 := connectAction.Execute(ctx, depsAny)
				Expect(err2).To(BeNil(), "second execution should succeed (skeleton implementation)")

				err3 := connectAction.Execute(ctx, depsAny)
				Expect(err3).To(BeNil(), "third execution should succeed (skeleton implementation)")
			})
		})
	})

	Describe("Name", func() {
		It("should return the correct action name", func() {
			connectAction := action.NewConnectAction()

			Expect(connectAction.Name()).To(Equal(action.ConnectActionName))
		})
	})
})
