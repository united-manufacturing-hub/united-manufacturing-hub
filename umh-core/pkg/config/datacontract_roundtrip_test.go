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

package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// sections is the pair of on-disk sections, in either shape.
type sections struct {
	DataModels    []DataModelsConfig      `yaml:"dataModels"`
	DataContracts []DataContractYAMLEntry `yaml:"dataContracts"`
}

// FuzzDataContractRoundTrip asserts that neither conversion loses information.
//
// This is the same property ContractsAreLossless enforces at runtime, using the
// same comparator, so what the fuzzer finds here is exactly what would degrade a
// real config rather than a test-only approximation.
//
// The corpus seeds it, so every shape a real config can take is a starting point
// rather than something the fuzzer has to discover. Any input this finds should be
// committed as a permanent seed and the code fixed -- never the assertion.
func FuzzDataContractRoundTrip(f *testing.F) {
	seeds, err := filepath.Glob("testdata/corpus/*.yaml")
	if err != nil {
		f.Fatalf("globbing corpus: %v", err)
	}

	if len(seeds) == 0 {
		f.Fatal("no corpus files found; the property is only as good as its seeds")
	}

	for _, path := range seeds {
		data, err := os.ReadFile(path)
		if err == nil {
			f.Add(data)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var parsed sections
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return // not a config; nothing to assert
		}

		contracts, notices := AbsorbConfig(parsed.DataModels, parsed.DataContracts)

		// If the input itself was lossy -- a duplicate address, a definition carrying
		// bridges -- round-tripping it proves nothing. Inputs that merely warn are
		// still checked: an orphaned contract survives as a bare address and has to
		// round-trip like anything else.
		if FirstDrop(notices) != nil {
			return
		}

		if !ContractsAreLossless(contracts) {
			// Re-derive both sides so the failure names which direction broke.
			merged, mergedNotices := AbsorbConfig(nil, ToYAMLEntries(contracts))
			models, legacy := ToLegacyConfig(contracts)
			downgraded, legacyNotices := AbsorbConfig(models, LegacyEntries(legacy))

			t.Fatalf("round trip lost information\nbefore     %+v\n"+
				"merged     %+v (notices %+v)\ndowngraded %+v (notices %+v)",
				contracts, merged, mergedNotices, downgraded, legacyNotices)
		}
	})
}
