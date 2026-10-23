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

// Package validator provides architecture validation utilities for FSMv2.
//
// Contains AST-based validators that enforce FSMv2 architectural patterns.
// Used by architecture_test.go to validate code at test time.
//
// # Validators
//
// Validates the following:
//   - State structs (empty fields, base embedding, String/Reason methods)
//   - Action structs (stateless, context cancellation, no channels)
//   - Worker files (DeriveDesiredState purity, dependency validation)
//   - Snapshot files (CollectedAt timestamp, IsShutdownRequested method)
//
// # Pattern registry
//
// PatternRegistry contains explanations for each architectural pattern.
// When a violation is found, the registry provides context about the pattern
// and how to fix the violation.
//
// # Usage
//
// Call validators from architecture_test.go:
//
//	violations := validator.ValidateStateStructs(baseDir)
//	if len(violations) > 0 {
//	    message := validator.FormatViolations("State Violations", violations)
//	    Fail(message)
//	}
package validator
