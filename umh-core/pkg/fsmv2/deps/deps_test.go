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

package deps_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var _ = Describe("BaseDependencies", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewBaseDependencies", func() {
		It("should create a non-nil dependencies", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := deps.NewBaseDependencies(logger, nil, identity)
			Expect(dependencies).NotTo(BeNil())
		})

		It("should return the logger passed to constructor", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := deps.NewBaseDependencies(logger, nil, identity)
			// Logger will be enriched with worker context
			Expect(dependencies.GetLogger()).NotTo(BeNil())
		})

		It("should panic when logger is nil", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
			Expect(func() {
				deps.NewBaseDependencies(nil, nil, identity)
			}).To(Panic())
		})
	})

	Describe("ActionLogger", func() {
		It("should return a logger enriched with action context", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := deps.NewBaseDependencies(logger, nil, identity)
			actionLog := dependencies.ActionLogger("test-action")
			Expect(actionLog).NotTo(BeNil())
		})
	})

	Describe("Dependencies interface compliance", func() {
		It("should implement Dependencies interface", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := deps.NewBaseDependencies(logger, nil, identity)
			var _ deps.Dependencies = dependencies
		})
	})

	Describe("GetActionHistory", func() {
		It("should return nil when no action history has been set", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := deps.NewBaseDependencies(logger, nil, identity)
			history := dependencies.GetActionHistory()
			Expect(history).To(BeNil())
		})

		It("should return action history set by supervisor", func() {
			identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
			dependencies := deps.NewBaseDependencies(logger, nil, identity)

			// Supervisor sets action history (workers can only read)
			dependencies.SetActionHistory([]deps.ActionResult{
				{
					ActionType: "TestAction",
					Success:    true,
				},
			})

			history := dependencies.GetActionHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].ActionType).To(Equal("TestAction"))
		})
	})
})

var _ = Describe("ActionHistory Read-Only Pattern", func() {
	var (
		logger       *zap.SugaredLogger
		dependencies *deps.BaseDependencies
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
		dependencies = deps.NewBaseDependencies(logger, nil, identity)
	})

	Describe("GetActionHistory", func() {
		Context("when no action history has been set", func() {
			It("should return nil (not a recorder interface)", func() {
				// GetActionHistory should return []ActionResult, not ActionHistoryRecorder
				// When no history is set, it returns nil
				history := dependencies.GetActionHistory()
				Expect(history).To(BeNil())
			})
		})

		Context("when action history has been set by supervisor", func() {
			It("should return the action history slice", func() {
				// Supervisor sets action history via SetActionHistory
				testHistory := []deps.ActionResult{
					{
						Timestamp:  time.Now(),
						ActionType: "TestAction",
						Success:    true,
						Latency:    100 * time.Millisecond,
					},
				}
				dependencies.SetActionHistory(testHistory)

				// Worker reads via GetActionHistory
				history := dependencies.GetActionHistory()
				Expect(history).To(HaveLen(1))
				Expect(history[0].ActionType).To(Equal("TestAction"))
				Expect(history[0].Success).To(BeTrue())
			})

			It("should return the same slice on multiple reads", func() {
				testHistory := []deps.ActionResult{
					{ActionType: "Action1", Success: true},
					{ActionType: "Action2", Success: false, ErrorMsg: "test error"},
				}
				dependencies.SetActionHistory(testHistory)

				// Multiple reads should return the same data
				history1 := dependencies.GetActionHistory()
				history2 := dependencies.GetActionHistory()
				Expect(history1).To(Equal(history2))
				Expect(history1).To(HaveLen(2))
			})
		})
	})

	Describe("SetActionHistory", func() {
		It("should allow supervisor to inject action history", func() {
			testHistory := []deps.ActionResult{
				{ActionType: "SupervisorRecordedAction", Success: true},
			}

			// This method is for supervisor use only
			dependencies.SetActionHistory(testHistory)

			history := dependencies.GetActionHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].ActionType).To(Equal("SupervisorRecordedAction"))
		})

		It("should replace previous action history on each call", func() {
			// First injection
			dependencies.SetActionHistory([]deps.ActionResult{
				{ActionType: "First", Success: true},
			})

			// Second injection (replaces)
			dependencies.SetActionHistory([]deps.ActionResult{
				{ActionType: "Second", Success: false},
				{ActionType: "Third", Success: true},
			})

			history := dependencies.GetActionHistory()
			Expect(history).To(HaveLen(2))
			Expect(history[0].ActionType).To(Equal("Second"))
			Expect(history[1].ActionType).To(Equal("Third"))
		})

		It("should allow setting nil to clear history", func() {
			dependencies.SetActionHistory([]deps.ActionResult{
				{ActionType: "ToBeCleared", Success: true},
			})

			dependencies.SetActionHistory(nil)

			history := dependencies.GetActionHistory()
			Expect(history).To(BeNil())
		})
	})

	Describe("Pattern consistency with FrameworkMetrics", func() {
		It("should follow the same getter/setter pattern as FrameworkMetrics", func() {
			// FrameworkMetrics: GetFrameworkState() returns *FrameworkMetrics, SetFrameworkState sets it
			// ActionHistory: GetActionHistory() returns []ActionResult, SetActionHistory sets it

			// Both start as nil
			Expect(dependencies.GetFrameworkState()).To(BeNil())
			Expect(dependencies.GetActionHistory()).To(BeNil())

			// Both can be set
			dependencies.SetFrameworkState(&deps.FrameworkMetrics{
				StateTransitionsTotal: 5,
			})
			dependencies.SetActionHistory([]deps.ActionResult{
				{ActionType: "TestAction", Success: true},
			})

			// Both can be read
			fm := dependencies.GetFrameworkState()
			Expect(fm).NotTo(BeNil())
			Expect(fm.StateTransitionsTotal).To(Equal(int64(5)))

			history := dependencies.GetActionHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].ActionType).To(Equal("TestAction"))
		})
	})
})

var _ = Describe("RetryTracker", func() {
	var (
		logger       *zap.SugaredLogger
		dependencies *deps.BaseDependencies
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		identity := deps.Identity{ID: "test-id", WorkerType: "test-worker"}
		dependencies = deps.NewBaseDependencies(logger, nil, identity)
	})

	Describe("RetryTracker()", func() {
		It("should return a non-nil tracker", func() {
			tracker := dependencies.RetryTracker()
			Expect(tracker).NotTo(BeNil())
		})

		It("should allow recording errors and reading consecutive errors", func() {
			tracker := dependencies.RetryTracker()

			Expect(tracker.ConsecutiveErrors()).To(Equal(0))

			tracker.RecordError()
			Expect(tracker.ConsecutiveErrors()).To(Equal(1))

			tracker.RecordError()
			Expect(tracker.ConsecutiveErrors()).To(Equal(2))
		})

		It("should reset consecutive errors on success", func() {
			tracker := dependencies.RetryTracker()

			tracker.RecordError()
			tracker.RecordError()
			Expect(tracker.ConsecutiveErrors()).To(Equal(2))

			tracker.RecordSuccess()
			Expect(tracker.ConsecutiveErrors()).To(Equal(0))
		})
	})
})

var _ = Describe("Identity", func() {
	Describe("String()", func() {
		It("returns HierarchyPath when available", func() {
			id := deps.Identity{
				ID:            "comm-001",
				WorkerType:    "communicator",
				HierarchyPath: "app/comm-001",
			}
			Expect(id.String()).To(Equal("app/comm-001"))
		})

		It("falls back to ID(Type) for root workers without HierarchyPath", func() {
			id := deps.Identity{
				ID:         "comm-001",
				WorkerType: "communicator",
			}
			Expect(id.String()).To(Equal("comm-001(communicator)"))
		})

		It("returns unknown when identity is empty", func() {
			id := deps.Identity{}
			Expect(id.String()).To(Equal("unknown"))
		})

		It("returns unknown when only ID is set", func() {
			id := deps.Identity{ID: "comm-001"}
			Expect(id.String()).To(Equal("unknown"))
		})

		It("returns unknown when only WorkerType is set", func() {
			id := deps.Identity{WorkerType: "communicator"}
			Expect(id.String()).To(Equal("unknown"))
		})

		It("prefers HierarchyPath over ID(Type) fallback", func() {
			id := deps.Identity{
				ID:            "comm-001",
				Name:          "Communicator Worker",
				WorkerType:    "communicator",
				HierarchyPath: "scenario123(application)/comm-001(communicator)",
			}
			Expect(id.String()).To(Equal("scenario123(application)/comm-001(communicator)"))
		})
	})
})
