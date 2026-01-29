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

package exampleparent_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

var _ = Describe("ParentDependencies", func() {
	var (
		logger     *zap.SugaredLogger
		parentDeps *exampleparent.ParentDependencies
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewParentDependencies", func() {
		It("should create dependencies with valid inputs", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "parent"}
			parentDeps = exampleparent.NewParentDependencies(logger, nil, identity)

			Expect(parentDeps).NotTo(BeNil())
			// Logger is enriched with worker context
			Expect(parentDeps.GetLogger()).NotTo(BeNil())
		})
	})
})
