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

package supervisor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("Child spec variables", func() {
	var (
		ctx   context.Context
		store *mockTriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockTriangularStore()
	})

	It("reaches the child exactly as the parent's ChildSpec set it", func() {
		specs := []config.ChildSpec{{
			Name:       "child1",
			WorkerType: "child",
			Enabled:    true,
			UserSpec: config.UserSpec{
				Config:    "child-config",
				Variables: map[string]any{"A": "1"},
			},
		}}

		s := newParentSupervisorWithSpecs(ctx, store, specs)
		s.TestUpdateUserSpec(config.UserSpec{
			Config:    "parent-config",
			Variables: map[string]any{"PARENT": "from-parent"},
		})

		err := s.TestTick(ctx)
		Expect(err).NotTo(HaveOccurred())

		children := s.GetChildren()
		Expect(children).To(HaveKey("child1"), "the tick must spawn the declared child")

		childSpec := children["child1"].TestGetUserSpec()
		Expect(childSpec).To(Equal(config.UserSpec{
			Config:    "child-config",
			Variables: map[string]any{"A": "1"},
		}), "the child's UserSpec must equal its ChildSpec's UserSpec, without the parent's variables")
	})
})
