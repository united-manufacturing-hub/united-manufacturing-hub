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

package examples_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("ScenarioDump rendering", func() {
	// Past 80 characters, the width at which the delta history truncates. The
	// cpu worker's health message is the real value that decided the rule.
	const longValue = "CPU contention. Tasks in this instance spent 25% of the last minute waiting for CPU time, over the 20% mark."

	It("truncates a long value in the delta history but renders it whole in FINAL STATE", func() {
		dump := &examples.ScenarioDump{
			EndSyncID: 1,
			Deltas: []storage.Delta{{
				WorkerType: "cpu",
				WorkerID:   "cpu-001",
				Role:       "observed",
				Changes: &storage.Diff{
					Modified: map[string]storage.ModifiedField{
						"message": {Old: "CPU: starting up.", New: longValue},
					},
				},
			}},
			Workers: []examples.WorkerSnapshot{{
				WorkerType: "cpu",
				WorkerID:   "cpu-001",
				Observed:   persistence.Document{"message": longValue},
			}},
		}

		history, finalState, found := strings.Cut(dump.FormatHuman(), "FINAL STATE")
		Expect(found).To(BeTrue(),
			"FormatHuman must emit a FINAL STATE section, otherwise the split below compares nothing")

		// The two sections make opposite trades and the guard pins both.
		// Asserting only the widening would still pass if FINAL STATE's full
		// width leaked into the history and made a long run unscannable.
		Expect(history).To(ContainSubstring(longValue[:77]+"..."),
			"the delta history must keep truncating at 80 characters")
		Expect(history).NotTo(ContainSubstring(longValue),
			"the delta history must not carry the whole value")
		Expect(finalState).To(ContainSubstring(longValue),
			"FINAL STATE must render the value whole: it is the only part of the dump where the full text is readable")
	})
})
