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
	"errors"
	"reflect"
	"sort"
)

// errContractsNotLossless is what a degraded config reports. Callers compare
// against it with errors.Is to distinguish "we refuse to touch this" from an
// ordinary write failure.
var errContractsNotLossless = errors.New(
	"data contracts cannot be converted without losing information; " +
		"config left in its existing shape and contract changes refused")

// LegacyEntries re-expresses pre-merge contracts as section entries, which is how
// a file written before the merge decodes. Used by the self-check and by tests
// that need to feed the pre-merge shape back through AbsorbConfig.
func LegacyEntries(legacy []DataContractsConfig) []DataContractYAMLEntry {
	entries := make([]DataContractYAMLEntry, 0, len(legacy))

	for _, c := range legacy {
		entries = append(entries, DataContractYAMLEntry{
			Name:           c.Name,
			LegacyModelRef: c.Model,
			DefaultBridges: c.DefaultBridges,
		})
	}

	return entries
}

// FirstWarning returns the first warn-level notice, or nil.
func FirstWarning(notices []MigrationNotice) *MigrationNotice {
	for i := range notices {
		if notices[i].Level == NoticeWarn {
			return &notices[i]
		}
	}

	return nil
}

// FirstDrop returns the first notice that actually discarded something, or nil.
//
// This, not FirstWarning, is what the round-trip property keys on. A config can
// warn loudly without losing anything -- an orphaned contract is kept as a bare
// address and still round-trips -- and gating on the level would skip exactly those
// inputs, which are the ones most worth checking.
func FirstDrop(notices []MigrationNotice) *MigrationNotice {
	for i := range notices {
		if notices[i].Dropped {
			return &notices[i]
		}
	}

	return nil
}

// ContractsEqual compares contract sets ignoring order and treating a nil map or
// slice as equal to an empty one.
//
// Both allowances are deliberate. Order is not part of what a contract set means,
// and nil-versus-empty is an artefact of how YAML decodes an absent key --
// asserting on either would make the round trip fail for reasons that are not
// information loss, which is the only thing it exists to detect.
func ContractsEqual(a, b []DataContract) bool {
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

// ContractsAreLossless reports whether these contracts survive both write paths.
//
// This runs on every parse, before anything is allowed to rewrite the file, and
// it is the guard that makes overwriting a user's config defensible: if either
// conversion cannot reproduce the contracts it was given, the file is left alone
// rather than replaced with a shape we cannot prove equivalent.
//
// Both directions are checked because both get written:
//
//   - the merged shape, which writeConfig emits
//   - the pre-merge shape, which downgrade-config emits and which an older
//     release has to be able to read
//
// A drop on either pass counts as failure. On this input the contracts came out of
// AbsorbConfig already, so anything it wants to discard the second time is
// something our own output cannot express.
func ContractsAreLossless(contracts []DataContract) bool {
	merged, mergedNotices := AbsorbConfig(nil, ToYAMLEntries(contracts))
	if FirstDrop(mergedNotices) != nil || !ContractsEqual(contracts, merged) {
		return false
	}

	models, legacy := ToLegacyConfig(contracts)

	downgraded, legacyNotices := AbsorbConfig(models, LegacyEntries(legacy))
	if FirstDrop(legacyNotices) != nil || !ContractsEqual(contracts, downgraded) {
		return false
	}

	return true
}

// LegacyDataModels projects the merged contracts back into the pre-merge models
// shape.
//
// This exists so consumers that legitimately still think in data models -- the
// structure validator, the enriched-tree walk -- have one sanctioned way to get
// them. Reading FullConfig.DataModels instead would work until the first mutation,
// at which point that field still holds whatever was last read from disk.
func (c FullConfig) LegacyDataModels() []DataModelsConfig {
	models, _ := ToLegacyConfig(c.Contracts)

	return models
}

// LegacyDataModelIndex is LegacyDataModels keyed by name, which is the shape both
// the validator and the reference walk actually want.
func (c FullConfig) LegacyDataModelIndex() map[string]DataModelsConfig {
	models := c.LegacyDataModels()
	index := make(map[string]DataModelsConfig, len(models))

	for _, model := range models {
		index[model.Name] = model
	}

	return index
}

// withContractsProjected returns the config as it should be written: contracts
// rendered into the merged section, the pre-merge models section dropped.
//
// A degraded config is returned untouched. That is the whole point of the flag --
// if the conversion is not provably lossless we would rather leave a file in the
// old shape than replace it with one we cannot vouch for. Mutations are refused
// separately, since persisting a change into sections we are no longer deriving
// from would lose it silently.
func (c FullConfig) withContractsProjected() FullConfig {
	if c.ContractsDegraded {
		return c
	}

	c.DataModels = nil
	c.DataContracts = ToYAMLEntries(c.Contracts)

	return c
}
