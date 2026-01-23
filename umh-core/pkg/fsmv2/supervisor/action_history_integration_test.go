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
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var _ = Describe("ActionHistory Integration", func() {
	Describe("ActionExecutor OnActionComplete callback", func() {
		It("should have an onActionComplete callback field", func() {
			// This test verifies the ActionExecutor has the callback mechanism
			// The implementation should add:
			// - onActionComplete func(deps.ActionResult) field to ActionExecutor
			// - SetOnActionComplete method to set the callback
			Skip("Implementation pending - ActionExecutor needs onActionComplete callback")
		})

		It("should call the callback after successful action execution", func() {
			Skip("Implementation pending - ActionExecutor needs onActionComplete callback")
		})

		It("should call the callback after failed action execution with error message", func() {
			Skip("Implementation pending - ActionExecutor needs onActionComplete callback")
		})

		It("should call the callback after action timeout", func() {
			Skip("Implementation pending - ActionExecutor needs onActionComplete callback")
		})

		It("should call the callback after action panic", func() {
			Skip("Implementation pending - ActionExecutor needs onActionComplete callback")
		})
	})

	Describe("WorkerContext actionHistory field", func() {
		It("should have an actionHistory field to store recorded actions", func() {
			// This test verifies WorkerContext has the actionHistory buffer
			// The implementation should add:
			// - actionHistory *deps.InMemoryActionHistoryRecorder field to WorkerContext
			Skip("Implementation pending - WorkerContext needs actionHistory field")
		})
	})

	Describe("ActionHistoryProvider and ActionHistorySetter wiring in AddWorker", func() {
		It("should wire up ActionHistoryProvider to drain from workerCtx.actionHistory", func() {
			// This test verifies the provider callback is set in CollectorConfig
			Skip("Implementation pending - AddWorker needs to wire ActionHistoryProvider")
		})

		It("should wire up ActionHistorySetter to inject into worker deps", func() {
			// This test verifies the setter callback is set in CollectorConfig
			Skip("Implementation pending - AddWorker needs to wire ActionHistorySetter")
		})
	})

	Describe("End-to-end ActionHistory flow", func() {
		It("should auto-record action results and make them available via deps.GetActionHistory()", func() {
			// Full integration test:
			// 1. Create supervisor with worker
			// 2. Execute an action
			// 3. Trigger collection cycle
			// 4. Verify deps.GetActionHistory() returns the recorded action
			Skip("Implementation pending - Full integration test")
		})
	})
})

// ActionResultMatcher helps verify ActionResult fields in tests.
type ActionResultMatcher struct {
	ActionType string
	Success    bool
	HasError   bool
}

func (m ActionResultMatcher) Match(actual interface{}) (success bool, err error) {
	result, ok := actual.(deps.ActionResult)
	if !ok {
		return false, nil
	}

	if result.ActionType != m.ActionType {
		return false, nil
	}

	if result.Success != m.Success {
		return false, nil
	}

	if m.HasError && result.ErrorMsg == "" {
		return false, nil
	}

	if !m.HasError && result.ErrorMsg != "" {
		return false, nil
	}
	// Verify timestamp is recent (within last minute)
	if time.Since(result.Timestamp) > time.Minute {
		return false, nil
	}

	return true, nil
}

func (m ActionResultMatcher) FailureMessage(actual interface{}) string {
	return "Expected ActionResult to match criteria"
}

func (m ActionResultMatcher) NegatedFailureMessage(actual interface{}) string {
	return "Expected ActionResult NOT to match criteria"
}
