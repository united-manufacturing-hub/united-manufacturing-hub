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
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"

	. "github.com/onsi/ginkgo/v2"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/validator"

	// Import state packages to scan for violations.
	childState "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/state"
	parentState "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/state"
)

var _ = Describe("FSMv2 Architecture Validation", func() {

	Context("🔴 PHASE 1: Architectural Validation Tests (Steps 52-55)", func() {

		Describe("Empty State Structs (Invariant: States are Pure Behavior)", func() {
			It("should have no fields except embedded base types", func() {
				violations := validateStateStructs()

				if len(violations) > 0 {
					message := validator.FormatViolations("Empty State Violations", violations)
					Fail(message)
				}
			})
		})

		Describe("Stateless Actions (Invariant: Actions are Idempotent Commands)", func() {
			It("should have no mutable state fields", func() {
				violations := validator.ValidateActionStructs(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolations("Stateless Action Violations", violations)
					Fail(message)
				}
			})
		})

		Describe("Type Assertions in Next() Methods (Invariant: Single Entry-Point Pattern)", func() {
			It("should have exactly one type assertion at method entry", func() {
				violations := validator.ValidateNextMethodTypeAssertions(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolations("Type Assertion Pattern Violations", violations)
					Fail(message)
				}
			})
		})

		Describe("Pure DeriveDesiredState() Methods (Invariant: No External Dependencies)", func() {
			It("should not access dependencies directly", func() {
				violations := validator.ValidateDeriveDesiredState(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolations("DeriveDesiredState Violations", violations)
					Fail(message)
				}
			})
		})

		Describe("Shutdown Check First in Next() (Invariant: Lifecycle Overrides Operational)", func() {
			It("should check IsShutdownRequested as first conditional after type assertion", func() {
				violations := validator.ValidateShutdownCheckFirst(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Shutdown Check Violations", violations, "SHUTDOWN_CHECK_NOT_FIRST")
					Fail(message)
				}
			})
		})

		Describe("State Change XOR Action (Invariant: Deterministic Transitions)", func() {
			It("should return either a new state or an action, never both", func() {
				violations := validator.ValidateStateXORAction(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("State XOR Action Violations", violations, "STATE_AND_ACTION")
					Fail(message)
				}
			})
		})

		Describe("ObservedState CollectedAt Timestamp (Invariant: Staleness Detection)", func() {
			It("should have CollectedAt time.Time field", func() {
				violations := validator.ValidateObservedStateTimestamp(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("CollectedAt Timestamp Violations", violations, "MISSING_COLLECTED_AT")
					Fail(message)
				}
			})
		})

		Describe("DesiredState IsShutdownRequested Method (Invariant: Graceful Shutdown)", func() {
			It("should implement IsShutdownRequested() bool", func() {
				violations := validator.ValidateDesiredStateShutdownMethod(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("IsShutdownRequested Method Violations", violations, "MISSING_IS_SHUTDOWN_REQUESTED")
					Fail(message)
				}
			})
		})

		Describe("State String() and Reason() Methods (Invariant: Debuggability)", func() {
			It("should have both String() and Reason() methods", func() {
				violations := validator.ValidateStateStringAndReason(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("State Method Violations", violations, "STATE_MISSING_STRING_METHOD")
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

		Describe("Context Cancellation in CollectObservedState (Invariant: Responsive Shutdown)", func() {
			It("should handle ctx.Done() for cancellation", func() {
				violations := validator.ValidateContextCancellationInCollect(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Context Cancellation Violations", violations, "MISSING_CONTEXT_CANCELLATION_COLLECT")
					Fail(message)
				}
			})
		})

		Describe("Nil Spec Handling in DeriveDesiredState (Invariant: Defensive Programming)", func() {
			It("should check if spec == nil before type casting", func() {
				violations := validator.ValidateNilSpecHandling(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Nil Spec Handling Violations", violations, "MISSING_NIL_SPEC_CHECK")
					Fail(message)
				}
			})
		})

		Describe("Context Cancellation in Actions (Invariant: Abortable Operations)", func() {
			It("should check ctx.Done() in Execute() methods", func() {
				violations := validator.ValidateContextCancellationInActions(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Action Context Violations", violations, "MISSING_CONTEXT_CANCELLATION_ACTION")
					Fail(message)
				}
			})
		})

		Describe("No Internal Retry Loops in Actions (Invariant: Supervisor Manages Retries)", func() {
			It("should not have for loops with error handling in Execute()", func() {
				violations := validator.ValidateNoInternalRetryLoops(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Internal Retry Loop Violations", violations, "INTERNAL_RETRY_LOOP")
					Fail(message)
				}
			})
		})

		Describe("Pointer Receivers on Workers (Invariant: Consistent Interface)", func() {
			It("should use pointer receivers (*T) for all Worker methods", func() {
				violations := validator.ValidatePointerReceivers(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Pointer Receiver Violations", violations, "VALUE_RECEIVER_ON_WORKER")
					Fail(message)
				}
			})
		})
	})

	Context("🟡 PHASE 2: Additional Architectural Patterns", func() {

		Describe("TryingTo States Return Actions (Invariant: Active State Semantics)", func() {
			It("should return non-nil actions in at least one code path", func() {
				violations := validator.ValidateTryingToStatesReturnActions(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("TryingTo State Action Violations", violations, "TRYINGTO_NO_ACTION")
					Fail(message)
				}
			})
		})

		Describe("Exhaustive Transition Coverage (Invariant: Complete State Handling)", func() {
			It("should end with catch-all return: return s, SignalNone, nil", func() {
				violations := validator.ValidateExhaustiveTransitionCoverage(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Catch-All Return Violations", violations, "MISSING_CATCHALL_RETURN")
					Fail(message)
				}
			})
		})

		Describe("Base State Type Embedding (Invariant: Type Hierarchy)", func() {
			It("should embed exactly one Base*State type", func() {
				violations := validator.ValidateBaseStateEmbedding(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Base State Embedding Violations", violations, "MISSING_BASE_STATE")
					Fail(message)
				}
			})
		})

		Describe("Dependency Validation in Constructors (Invariant: Fail-Fast)", func() {
			It("should validate required dependencies are non-nil", func() {
				violations := validator.ValidateDependencyValidation(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Dependency Validation Violations", violations, "MISSING_DEPENDENCY_VALIDATION")
					Fail(message)
				}
			})
		})

		Describe("Child Spec Validation (Invariant: Early Validation)", func() {
			It("should validate ChildrenSpecs before returning them", func() {
				violations := validator.ValidateChildSpecValidation(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Child Spec Validation Violations", violations, "MISSING_CHILDSPEC_VALIDATION")
					Fail(message)
				}
			})
		})

		Describe("No Channel Operations in Actions (Invariant: Synchronous Actions)", func() {
			It("should not use goroutines, channels, or channel operations", func() {
				violations := validator.ValidateNoChannelOperations(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Channel Operation Violations", violations, "CHANNEL_OPERATION_IN_ACTION")
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

// validateStateStructs checks that state structs have no fields (except base state).
// This function must remain in the test file because it uses reflection on imported
// state packages, which would create import cycles if moved to the validator package.
func validateStateStructs() []validator.Violation {
	var violations []validator.Violation

	fsmv2Dir := getFsmv2Dir()

	// Check example-child states
	childStates := []interface{}{
		&childState.TryingToConnectState{},
		&childState.ConnectedState{},
		&childState.DisconnectedState{},
		&childState.TryingToStopState{},
		&childState.StoppedState{},
	}

	for _, state := range childStates {
		v := reflect.ValueOf(state).Elem()
		t := v.Type()

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			// Skip embedded base types (they start with capital letter and are anonymous)
			if field.Anonymous {
				continue
			}

			// Found a violation: state has a field
			violations = append(violations, validator.Violation{
				File:    validator.GetStateFilePath(fsmv2Dir, t.Name()),
				Line:    0, // Would need source parsing to get exact line
				Type:    "EMPTY_STATE",
				Message: fmt.Sprintf("State %s has field '%s %s' (should be empty)", t.Name(), field.Name, field.Type),
			})
		}
	}

	// Check example-parent states
	parentStates := []interface{}{
		&parentState.TryingToStartState{},
		&parentState.RunningState{},
		&parentState.TryingToStopState{},
		&parentState.StoppedState{},
	}

	for _, state := range parentStates {
		v := reflect.ValueOf(state).Elem()
		t := v.Type()

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			if field.Anonymous {
				continue
			}

			violations = append(violations, validator.Violation{
				File:    validator.GetStateFilePath(fsmv2Dir, t.Name()),
				Line:    0,
				Type:    "EMPTY_STATE",
				Message: fmt.Sprintf("State %s has field '%s %s' (should be empty)", t.Name(), field.Name, field.Type),
			})
		}
	}

	return violations
}
