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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var _ = Describe("ActionHistory Read-Only Pattern", func() {
	var (
		logger *zap.SugaredLogger
		deps   *fsmv2.BaseDependencies
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		identity := fsmv2.Identity{ID: "test-id", WorkerType: "test-worker"}
		deps = fsmv2.NewBaseDependencies(logger, nil, identity)
	})

	Describe("GetActionHistory", func() {
		Context("when no action history has been set", func() {
			It("should return nil (not a recorder interface)", func() {
				// GetActionHistory should return []ActionResult, not ActionHistoryRecorder
				// When no history is set, it returns nil
				history := deps.GetActionHistory()
				Expect(history).To(BeNil())
			})
		})

		Context("when action history has been set by supervisor", func() {
			It("should return the action history slice", func() {
				// Supervisor sets action history via SetActionHistory
				testHistory := []fsmv2.ActionResult{
					{
						Timestamp:  time.Now(),
						ActionType: "TestAction",
						Success:    true,
						Latency:    100 * time.Millisecond,
					},
				}
				deps.SetActionHistory(testHistory)

				// Worker reads via GetActionHistory
				history := deps.GetActionHistory()
				Expect(history).To(HaveLen(1))
				Expect(history[0].ActionType).To(Equal("TestAction"))
				Expect(history[0].Success).To(BeTrue())
			})

			It("should return the same slice on multiple reads", func() {
				testHistory := []fsmv2.ActionResult{
					{ActionType: "Action1", Success: true},
					{ActionType: "Action2", Success: false, ErrorMsg: "test error"},
				}
				deps.SetActionHistory(testHistory)

				// Multiple reads should return the same data
				history1 := deps.GetActionHistory()
				history2 := deps.GetActionHistory()
				Expect(history1).To(Equal(history2))
				Expect(history1).To(HaveLen(2))
			})
		})
	})

	Describe("SetActionHistory", func() {
		It("should allow supervisor to inject action history", func() {
			testHistory := []fsmv2.ActionResult{
				{ActionType: "SupervisorRecordedAction", Success: true},
			}

			// This method is for supervisor use only
			deps.SetActionHistory(testHistory)

			history := deps.GetActionHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].ActionType).To(Equal("SupervisorRecordedAction"))
		})

		It("should replace previous action history on each call", func() {
			// First injection
			deps.SetActionHistory([]fsmv2.ActionResult{
				{ActionType: "First", Success: true},
			})

			// Second injection (replaces)
			deps.SetActionHistory([]fsmv2.ActionResult{
				{ActionType: "Second", Success: false},
				{ActionType: "Third", Success: true},
			})

			history := deps.GetActionHistory()
			Expect(history).To(HaveLen(2))
			Expect(history[0].ActionType).To(Equal("Second"))
			Expect(history[1].ActionType).To(Equal("Third"))
		})

		It("should allow setting nil to clear history", func() {
			deps.SetActionHistory([]fsmv2.ActionResult{
				{ActionType: "ToBeCleared", Success: true},
			})

			deps.SetActionHistory(nil)

			history := deps.GetActionHistory()
			Expect(history).To(BeNil())
		})
	})

	Describe("Pattern consistency with FrameworkMetrics", func() {
		It("should follow the same getter/setter pattern as FrameworkMetrics", func() {
			// FrameworkMetrics: GetFrameworkState() returns *FrameworkMetrics, SetFrameworkState sets it
			// ActionHistory: GetActionHistory() returns []ActionResult, SetActionHistory sets it

			// Both start as nil
			Expect(deps.GetFrameworkState()).To(BeNil())
			Expect(deps.GetActionHistory()).To(BeNil())

			// Both can be set
			deps.SetFrameworkState(&fsmv2.FrameworkMetrics{
				StateTransitionsTotal: 5,
			})
			deps.SetActionHistory([]fsmv2.ActionResult{
				{ActionType: "TestAction", Success: true},
			})

			// Both can be read
			fm := deps.GetFrameworkState()
			Expect(fm).NotTo(BeNil())
			Expect(fm.StateTransitionsTotal).To(Equal(int64(5)))

			history := deps.GetActionHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].ActionType).To(Equal("TestAction"))
		})
	})
})
