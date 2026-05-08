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
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/validator"
)

var _ = Describe("FSMv2 Architecture Validation", func() {

	Context("🔴 PHASE 1: Architectural Validation Tests (Steps 52-55)", func() {

		Describe("State Change XOR Action (Invariant: Deterministic Transitions)", func() {
			It("should return either a new state or an action, never both", func() {
				violations := validator.ValidateStateXORAction(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("State XOR Action Violations", violations, "STATE_AND_ACTION")
					Fail(message)
				}
			})
		})

		Describe("No Nil State Returns (Invariant: Non-Nil State Guarantee)", func() {
			It("should never return nil as state in Next()", func() {
				violations := validator.ValidateNoNilStateReturns(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Nil State Violations", violations, "NIL_STATE_RETURN")
					Fail(message)
				}
			})
		})

		Describe("Signal-State Mutual Exclusion (Invariant: Clear Signal Semantics)", func() {
			It("should only send signals with same-state returns", func() {
				violations := validator.ValidateSignalStateMutualExclusion(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Signal State Violations", violations, "SIGNAL_STATE_MISMATCH")
					Fail(message)
				}
			})
		})
	})

	Context("🟡 PHASE 2: Additional Architectural Patterns", func() {

		Describe("StoppingState No Catch-All Self-Return (Invariant: No Stopping Deadlock)", func() {
			It("should not have catch-all self-return with nil action in StoppingBase states", func() {
				violations := validator.ValidateStoppingStateNoCatchAllSelfReturn(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern(
						"StoppingState Deadlock Violations",
						violations,
						"STOPPING_STATE_DEADLOCK",
					)
					Fail(message)
				}
			})
		})
	})
})

// getFsmv2Dir returns the path to the fsmv2 package directory.
func getFsmv2Dir() string {
	_, filename, _, _ := runtime.Caller(0)

	return filepath.Dir(filename)
}
