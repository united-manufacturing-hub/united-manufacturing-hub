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
	"reflect"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

// sections is the pair of on-disk sections, in either shape.
type sections struct {
	DataModels    []DataModelsConfig      `yaml:"dataModels"`
	DataContracts []DataContractYAMLEntry `yaml:"dataContracts"`
}

// equalContracts compares contract sets ignoring order and treating a nil map or
// slice as equal to an empty one.
//
// Both allowances are deliberate. Order is not part of the meaning of a contract
// set, and nil-versus-empty is an artefact of how YAML decodes an absent key —
// asserting on either would make the property fail for reasons that are not
// information loss, which is what it exists to detect.
func equalContracts(a, b []DataContract) bool {
	if len(a) != len(b) {
		return false
	}

	key := func(c DataContract) string {
		return c.Name + "\x00" + c.Model + "\x00" + c.Version
	}

	sortByKey := func(in []DataContract) []DataContract {
		out := append([]DataContract(nil), in...)
		sort.Slice(out, func(i, j int) bool { return key(out[i]) < key(out[j]) })

		return out
	}

	left, right := sortByKey(a), sortByKey(b)

	for i := range left {
		l, r := left[i], right[i]

		if l.Name != r.Name || l.Model != r.Model ||
			l.Version != r.Version || l.Description != r.Description {
			return false
		}

		if len(l.Structure) != len(r.Structure) {
			return false
		}

		if len(l.Structure) > 0 && !reflect.DeepEqual(l.Structure, r.Structure) {
			return false
		}

		if len(l.DefaultBridges) != len(r.DefaultBridges) {
			return false
		}

		if len(l.DefaultBridges) > 0 && !reflect.DeepEqual(l.DefaultBridges, r.DefaultBridges) {
			return false
		}
	}

	return true
}

func hasWarning(notices []MigrationNotice) *MigrationNotice {
	for i := range notices {
		if notices[i].Level == NoticeWarn {
			return &notices[i]
		}
	}

	return nil
}

// FuzzDataContractRoundTrip asserts that neither conversion loses information.
//
// Two round trips matter and both are checked:
//
//   - through the merged shape, which is what gets written to disk
//   - through the pre-merge shape, which is what `downgrade-config` produces and
//     what an older release has to be able to read
//
// The corpus seeds it, so every shape a real config can take is a starting point
// rather than something the fuzzer has to discover. Any input this finds should be
// committed as a permanent seed and the code fixed — never the assertion.
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

		first, notices := AbsorbConfig(parsed.DataModels, parsed.DataContracts)

		// A warning on the first pass means the input itself was lossy — an orphan,
		// a duplicate address. Round-tripping that is not meaningful.
		if hasWarning(notices) != nil {
			return
		}

		t.Run("merged shape", func(t *testing.T) {
			again, notices := AbsorbConfig(nil, ToYAMLEntries(first))
			if w := hasWarning(notices); w != nil {
				t.Fatalf("merged round trip warned: %+v", *w)
			}

			if !equalContracts(first, again) {
				t.Fatalf("merged round trip lost information:\nbefore %+v\nafter  %+v", first, again)
			}
		})

		t.Run("legacy shape", func(t *testing.T) {
			models, legacy := ToLegacyConfig(first)

			entries := make([]DataContractYAMLEntry, 0, len(legacy))
			for _, c := range legacy {
				entries = append(entries, DataContractYAMLEntry{
					Name:           c.Name,
					LegacyModelRef: c.Model,
					DefaultBridges: c.DefaultBridges,
				})
			}

			again, notices := AbsorbConfig(models, entries)
			if w := hasWarning(notices); w != nil {
				t.Fatalf("legacy round trip warned: %+v", *w)
			}

			if !equalContracts(first, again) {
				t.Fatalf("legacy round trip lost information:\nbefore %+v\nafter  %+v", first, again)
			}
		})
	})
}
