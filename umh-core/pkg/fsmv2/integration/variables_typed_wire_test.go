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

package integration_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// VariablesInternal JSON round-trip — closes pr1_issues.md issue #7
// (OriginalUserSpec json:"-" wire-path coverage gap) by anchoring the
// stdlib `encoding/json` round-trip shape directly rather than via the
// OriginalUserSpec round-trip path (which is now json:"-" per P1.5c
// Row 3).
//
// Scope is the stdlib json marshal/unmarshal pair, not a custom typed
// wire-reconstruction codec. Naming reflects that: "JSON round-trip" is
// what the test exercises; "typed-wire" framing was misleading because
// nothing here exercises a typed-wire path beyond what `encoding/json`
// gives by default.
//
// The Skip'd specs in inheritance_scenario_test.go remain skipped — they
// need broader integration-level coverage (a separate workstream). This
// suite addresses the specific schema-stability concern issue #7 raised:
// ensure the wire shape locked in §4-D LOCKED survives a JSON round-trip
// without field loss.
//
// Per CLAUDE.md Testing Guidelines, FSMv2 uses Ginkgo v2 + Gomega; this
// suite follows the project convention.
var _ = Describe("VariablesInternal JSON round-trip (closes pr1_issues #7)", func() {
	It("populated VariablesInternal survives JSON round-trip without field loss", func() {
		src := config.VariablesInternal{
			WorkerID:  "worker-42",
			ParentID:  "parent-1",
			BridgedBy: "bridge-source-7",
			CreatedAt: time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC),
		}

		data, err := json.Marshal(src)
		Expect(err).NotTo(HaveOccurred(), "marshal VariablesInternal")

		var dst config.VariablesInternal
		Expect(json.Unmarshal(data, &dst)).To(Succeed(), "unmarshal VariablesInternal")

		Expect(dst.WorkerID).To(Equal(src.WorkerID), "WorkerID round-trip")
		Expect(dst.ParentID).To(Equal(src.ParentID), "ParentID round-trip")
		Expect(dst.BridgedBy).To(Equal(src.BridgedBy), "BridgedBy round-trip")
		Expect(dst.CreatedAt.Equal(src.CreatedAt)).To(BeTrue(),
			"CreatedAt round-trip: got %v want %v", dst.CreatedAt, src.CreatedAt)
	})

	It("JSON shape uses §4-D LOCKED tag spelling (workerID/parentID/bridgedBy/createdAt)", func() {
		// Catches accidental tag changes that would break the CSE wire
		// format and produce a delta-storm against pre-P1.5c documents.
		src := config.VariablesInternal{
			WorkerID:  "x",
			ParentID:  "y",
			BridgedBy: "z",
			CreatedAt: time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC),
		}
		data, err := json.Marshal(src)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]any
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		for _, key := range []string{"workerID", "parentID", "bridgedBy", "createdAt"} {
			Expect(raw).To(HaveKey(key),
				"JSON missing expected key %q (§4-D LOCKED schema)", key)
		}
	})

	It("omitempty tags drop ParentID and BridgedBy from the wire when empty", func() {
		emptySrc := config.VariablesInternal{
			WorkerID:  "worker-only",
			CreatedAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
		}
		data, err := json.Marshal(emptySrc)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]any
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		Expect(raw).NotTo(HaveKey("parentID"),
			"parentID should be omitted when empty (omitempty tag) but appeared in JSON: %s", string(data))
		Expect(raw).NotTo(HaveKey("bridgedBy"),
			"bridgedBy should be omitted when empty (omitempty tag) but appeared in JSON: %s", string(data))
		Expect(raw).To(HaveKey("workerID"), "workerID must always be present (no omitempty tag)")
		Expect(raw).To(HaveKey("createdAt"), "createdAt must always be present (no omitempty tag)")
	})
})
