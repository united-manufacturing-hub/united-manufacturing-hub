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


package fsmv2_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

func TestFsmv2(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FSMv2 Suite")
}

var _ = Describe("BaseDependencies", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewBaseDependencies", func() {
		It("should create a non-nil dependencies", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := fsmv2.NewBaseDependencies(logger, nil, identity)
			Expect(dependencies).NotTo(BeNil())
		})

		It("should return the logger passed to constructor", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := fsmv2.NewBaseDependencies(logger, nil, identity)
			// Logger will be enriched with worker context
			Expect(dependencies.GetLogger()).NotTo(BeNil())
		})

		It("should panic when logger is nil", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			Expect(func() {
				fsmv2.NewBaseDependencies(nil, nil, identity)
			}).To(Panic())
		})
	})

	Describe("ActionLogger", func() {
		It("should return a logger enriched with action context", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := fsmv2.NewBaseDependencies(logger, nil, identity)
			actionLog := dependencies.ActionLogger("test-action")
			Expect(actionLog).NotTo(BeNil())
		})
	})

	Describe("Dependencies interface compliance", func() {
		It("should implement Dependencies interface", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := fsmv2.NewBaseDependencies(logger, nil, identity)
			var _ fsmv2.Dependencies = dependencies
		})
	})

	Describe("GetActionHistory", func() {
		It("should return a non-nil ActionHistoryRecorder", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := fsmv2.NewBaseDependencies(logger, nil, identity)
			recorder := dependencies.GetActionHistory()
			Expect(recorder).NotTo(BeNil())
		})

		It("should allow recording action results", func() {
			identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := fsmv2.NewBaseDependencies(logger, nil, identity)
			recorder := dependencies.GetActionHistory()

			recorder.Record(fsmv2.ActionResult{
				ActionType: "TestAction",
				Success:    true,
			})

			results := recorder.Drain()
			Expect(results).To(HaveLen(1))
			Expect(results[0].ActionType).To(Equal("TestAction"))
		})
	})
})
