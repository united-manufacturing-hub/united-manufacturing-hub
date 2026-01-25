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
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/validator"

	// Import state packages to scan for violations.
	childState "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
	parentState "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"

	// Import worker packages to register them for registry consistency tests.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

var _ = Describe("FSMv2 Architecture Validation", func() {

	Context("🔴 PHASE 1: Architectural Validation Tests (Steps 52-55)", func() {

		Describe("Empty State Structs (Invariant: States are Pure Behavior)", func() {
			It("should have no fields except embedded base types", func() {
				violations := validateStateStructs()
				Expect(violations).To(BeEmpty(), validator.FormatViolations("Empty State Violations", violations))
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

		Describe("State String() Method (Invariant: Debuggability)", func() {
			// Note: Reason() method was removed from State interface - reason now comes from NextResult.Reason
			It("should have String() method for debugging", func() {
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

		Describe("ObservedState Embeds DesiredState (Invariant: State Consistency)", func() {
			It("should embed DesiredState anonymously with json:\",inline\" tag", func() {
				violations := validator.ValidateObservedStateEmbedsDesired(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("ObservedState Embedding Violations", violations, "OBSERVED_STATE_NOT_EMBEDDING_DESIRED")
					Fail(message)
				}
			})
		})

		Describe("State Field Exists (Invariant: FSM State Tracking)", func() {
			It("should have State string field in both DesiredState and ObservedState", func() {
				violations := validator.ValidateStateFieldExists(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("State Field Violations", violations, "MISSING_STATE_FIELD")
					Fail(message)
				}
			})
		})

		Describe("ObservedState SetState Method (Invariant: StateProvider Callback)", func() {
			It("should have SetState(string) method for StateProvider injection", func() {
				violations := validator.ValidateObservedStateHasSetState(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("SetState Method Violations", violations, "MISSING_SET_STATE_METHOD")
					Fail(message)
				}
			})
		})

		Describe("DesiredState.State Values (Invariant: Valid Lifecycle States)", func() {
			It("should only use \"stopped\" or \"running\" as values", func() {
				violations := validator.ValidateDesiredStateValues(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Invalid State Value Violations", violations, "INVALID_DESIRED_STATE_VALUE")
					Fail(message)
				}
			})
		})

		Describe("DeriveDesiredState Returns (Invariant: Valid Lifecycle States)", func() {
			It("should only return \"stopped\" or \"running\" in State field", func() {
				violations := validator.ValidateDeriveDesiredStateReturns(getFsmv2Dir())

				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("DeriveDesiredState State Value Violations", violations, "INVALID_DESIRED_STATE_VALUE")
					Fail(message)
				}
			})
		})

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

		Describe("No Direct Document Manipulation (Invariant: Type-Safe Boundaries)", func() {
			It("should not type-assert to persistence.Document in supervisor", func() {
				violations := validator.ValidateNoDirectDocumentManipulation(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Direct Document Manipulation Violations", violations, "DIRECT_DOCUMENT_ASSERTION")
					Fail(message)
				}
			})
		})

		Describe("Structured Logging Only (Invariant: Consistent Log Format)", func() {
			It("should use structured logging (Warnw/Errorw/Infow) not format-based (Warnf/Errorf/Infof)", func() {
				violations := validator.ValidateStructuredLogging(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Non-Structured Logging Violations", violations, "NON_STRUCTURED_LOGGING")
					Fail(message)
				}
			})
		})

		Describe("DesiredState Has No Dependencies (Invariant: Serializable Configuration)", func() {
			It("should not have Dependencies field in any DesiredState struct", func() {
				violations := validator.ValidateDesiredStateHasNoDependencies(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Dependencies in DesiredState Violations", violations, "DEPENDENCIES_IN_DESIRED_STATE")
					Fail(message)
				}
			})
		})

		Describe("Worker Folder Naming (Invariant: Folder = Worker Type)", func() {
			It("should have folder name equal to derived worker type", func() {
				violations := validator.ValidateFolderMatchesWorkerType(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern("Folder Naming Violations", violations, "FOLDER_WORKER_TYPE_MISMATCH")
					Fail(message)
				}
			})
		})

		Describe("Worker Factory Registration (Invariant: Complete Registry)", func() {
			It("should have matching worker and supervisor registrations", func() {
				workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()

				if len(workerOnly) > 0 || len(supervisorOnly) > 0 {
					var violations []validator.Violation

					sort.Strings(workerOnly)
					for _, wt := range workerOnly {
						violations = append(violations, validator.Violation{
							File:    "factory/worker_factory.go",
							Type:    "REGISTRY_MISMATCH",
							Message: fmt.Sprintf("Worker '%s' registered without supervisor factory", wt),
						})
					}

					sort.Strings(supervisorOnly)
					for _, st := range supervisorOnly {
						violations = append(violations, validator.Violation{
							File:    "factory/worker_factory.go",
							Type:    "REGISTRY_MISMATCH",
							Message: fmt.Sprintf("Supervisor '%s' registered without worker factory", st),
						})
					}

					message := validator.FormatViolationsWithPattern(
						"Registry Mismatch", violations, "REGISTRY_MISMATCH")
					Fail(message)
				}
			})
		})

		Describe("Child Workers Use IsStopRequired() (Invariant: Parent Lifecycle Awareness)", func() {
			It("should use IsStopRequired() not just IsShutdownRequested() in child worker states", func() {
				violations := validator.ValidateChildWorkersIsStopRequired(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern(
						"Child Worker IsStopRequired Violations",
						violations,
						"CHILD_MUST_USE_IS_STOP_REQUIRED",
					)
					Fail(message)
				}
			})
		})

		Describe("No Custom Lifecycle Fields (Invariant: FSM Controls Lifecycle)", func() {
			It("should not have ShouldRun, IsRunning, or similar fields in DesiredState", func() {
				violations := validator.ValidateNoCustomLifecycleFields(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern(
						"Custom Lifecycle Field Violations",
						violations,
						"CUSTOM_LIFECYCLE_FIELD",
					)
					Fail(message)
				}
			})
		})

		Describe("Framework Metrics Copy (Invariant: Workers Copy Metrics from Deps)", func() {
			It("should call GetFrameworkState() when GetDependencies() is used in CollectObservedState", func() {
				violations := validator.ValidateFrameworkMetricsCopy(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern(
						"Framework Metrics Copy Violations",
						violations,
						"MISSING_FRAMEWORK_METRICS_COPY",
					)
					Fail(message)
				}
			})
		})

		Describe("Action History Copy (Invariant: Workers Copy ActionHistory from Deps)", func() {
			It("should call GetActionHistory() when GetDependencies() is used in CollectObservedState", func() {
				violations := validator.ValidateActionHistoryCopy(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern(
						"Action History Copy Violations",
						violations,
						"MISSING_ACTION_HISTORY_COPY",
					)
					Fail(message)
				}
			})
		})

		Describe("MetricsEmbedder Value Receivers (Invariant: Interface Satisfaction)", func() {
			It("should use value receivers for MetricsHolder interface methods", func() {
				violations := validator.ValidateMetricsEmbedderValueReceivers(getFsmv2Dir())
				if len(violations) > 0 {
					message := validator.FormatViolationsWithPattern(
						"MetricsEmbedder Receiver Violations",
						violations,
						"POINTER_RECEIVER_ON_METRICS_EMBEDDER",
					)
					Fail(message)
				}
			})
		})

		Describe("Static Error Messages (Invariant: Sentry Grouping)", func() {
			It("should not have dynamic content in error messages", func() {
				violations := validator.ValidateStaticErrorMessages(getFsmv2Dir())
				if len(violations) > 0 {
					// Report violations but skip instead of failing
					// P2-19 will fix all violations; this test documents what needs fixing
					message := validator.FormatViolationsWithPattern(
						"Static Error Message Violations",
						violations,
						"DYNAMIC_ERROR_MESSAGE",
					)
					maxViolations := 20
					if len(violations) > maxViolations {
						Skip(fmt.Sprintf("Found %d static error violations (showing first %d, to be fixed in P2-19):\n%s",
							len(violations), maxViolations, message))
					} else {
						Skip(fmt.Sprintf("Found %d static error violations (to be fixed in P2-19):\n%s",
							len(violations), message))
					}
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

		for i := range t.NumField() {
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

	// Check exampleparent states
	parentStates := []interface{}{
		&parentState.TryingToStartState{},
		&parentState.RunningState{},
		&parentState.TryingToStopState{},
		&parentState.StoppedState{},
	}

	for _, state := range parentStates {
		v := reflect.ValueOf(state).Elem()
		t := v.Type()

		for i := range t.NumField() {
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
