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

import (
	"path/filepath"
	"testing"
)

// TestIsChildWorkerDetectsKnownChildWorkers anchors the isChildWorker heuristic
// against known child workers in the tree. Before P1.5b CHANGE-4 the heuristic
// scanned snapshot/snapshot.go for the literal "IsStopRequired()" string, which
// was already vacuous (worker observed-state methods live in snapshot.go but
// the rule we care about is "do state files use the parent-aware stop check").
// The renamed heuristic scans state/*.go for ParentMappedState reads or
// ShouldStop() calls. This test guards against future regressions to a
// vacuous form in either direction:
//   - always-false (the pre-P1.5b bug): positive cases below would fail
//   - always-true (the symmetric bug): negative cases below would fail
func TestIsChildWorkerDetectsKnownChildWorkers(t *testing.T) {
	// fsmv2 root relative to internal/validator/.
	fsmv2Root := filepath.Join("..", "..")

	// Positive cases: every known child worker in the tree. Coverage is
	// load-bearing — adding a new child worker without updating this list
	// silently weakens the regression guard.
	knownChildren := []string{
		filepath.Join(fsmv2Root, "workers", "example", "examplechild"),
		filepath.Join(fsmv2Root, "workers", "example", "examplefailing"),
		filepath.Join(fsmv2Root, "workers", "example", "exampleslow"),
		filepath.Join(fsmv2Root, "workers", "transport", "push"),
		filepath.Join(fsmv2Root, "workers", "transport", "pull"),
	}

	for _, path := range knownChildren {
		if !isChildWorker(path) {
			t.Errorf("isChildWorker(%q) = false; want true (regression — heuristic became vacuous always-false)", path)
		}
	}

	// Negative cases: known root/parent workers that must NOT be classified
	// as children. Catches a regression that always returns true.
	nonChildren := []string{
		filepath.Join(fsmv2Root, "workers", "example", "exampleparent"),
		filepath.Join(fsmv2Root, "workers", "example", "helloworld"),
	}

	for _, path := range nonChildren {
		if isChildWorker(path) {
			t.Errorf("isChildWorker(%q) = true; want false (regression — heuristic became vacuous always-true)", path)
		}
	}
}
