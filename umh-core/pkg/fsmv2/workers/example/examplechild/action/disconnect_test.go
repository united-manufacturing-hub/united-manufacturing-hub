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
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/action"
)

var _ = Describe("DisconnectAction", func() {
	var (
		ctx     context.Context
		depsAny any
	)

	BeforeEach(func() {
		ctx = context.Background()
		identity := deps.Identity{ID: "test-id", WorkerType: "child"}
		depsAny = example_child.NewExamplechildDependencies(&example_child.DefaultConnectionPool{}, zap.NewNop().Sugar(), nil, identity)
	})

	Describe("Execute", func() {
		It("should complete without error", func() {
			disconnectAction := &action.DisconnectAction{}

			err := disconnectAction.Execute(ctx, depsAny)

			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Name", func() {
		It("should return the correct action name", func() {
			disconnectAction := &action.DisconnectAction{}

			Expect(disconnectAction.Name()).To(Equal(action.DisconnectActionName))
		})
	})
})
