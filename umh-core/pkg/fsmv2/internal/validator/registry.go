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

package validator

// PatternRegistry maps pattern types to their WHY explanations.
// This registry provides educational context when violations are found.
var PatternRegistry = map[string]PatternInfo{
	"STATE_AND_ACTION": {
		Name: "State Change XOR Action",
		Why: `Next() must return EITHER a new state OR an action, never both simultaneously.
WHY: Actions execute asynchronously. If the state changes at the same time,
the supervisor doesn't know which state to transition to after the action
completes. This rule ensures deterministic behavior: stay in current state →
execute action → observe result → then decide on state transition.`,
		CorrectCode: `// Correct: state change, no action
return &ConnectedState{}, SignalNone, nil

// Correct: same state, with action
return s, SignalNone, &ConnectAction{}

// WRONG: both state change AND action
return &ConnectedState{}, SignalNone, &SomeAction{}`,
		ReferenceFile: "example-child/state/state_trying_to_connect.go:38-41",
	},
	"NIL_STATE_RETURN": {
		Name: "No Nil State Returns",
		Why: `Next() must NEVER return nil as the first return value (state).
WHY: The supervisor assumes states are always non-nil. A nil state causes a
panic when calling state.String() or state.Next(). States should always
return a valid state - either the current state (s) or a new state type.
If "no change" is needed, return s (current state), not nil.`,
		CorrectCode: `// Correct: stay in same state
return s, SignalNone, nil

// Correct: transition to different state
return &StoppedState{}, SignalNone, nil

// WRONG: nil state causes panic
return nil, SignalNone, nil`,
		ReferenceFile: "example-child/state/state_connected.go",
	},
	"SIGNAL_STATE_MISMATCH": {
		Name: "Signal-State Mutual Exclusion",
		Why: `When returning SignalNeeds* (removal/restart), the state must be the current state (s).
WHY: Signals indicate the worker needs external intervention (removal, restart).
If the state changes simultaneously, there's ambiguity: does the new state apply
before or after the signal is processed? By requiring signals only with same-state
returns, we ensure clear semantics: signal processing happens in the current state.`,
		CorrectCode: `// Correct: signal with same state
return s, SignalNeedsRemoval, nil

// Correct: state change without signal
return &StoppedState{}, SignalNone, nil

// WRONG: state change with signal
return &StoppedState{}, SignalNeedsRemoval, nil`,
		ReferenceFile: "example-child/state/state_stopped.go",
	},
}

// GetPattern returns pattern info for a violation type.
func GetPattern(patternType string) (PatternInfo, bool) {
	pattern, ok := PatternRegistry[patternType]

	return pattern, ok
}
