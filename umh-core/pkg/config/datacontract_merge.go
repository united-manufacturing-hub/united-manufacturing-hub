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
	"fmt"
	"sort"
)

// DataContract is one data contract: the address it is published under, the
// structure it enforces, and the lineage it belongs to.
//
// Three shapes occur, and all of them are real configs:
//
//   - Addressed   Name, Model, Version and Structure all set
//   - Definition  Name empty — a structure other contracts reference but nothing
//     publishes to, which is exactly its behaviour today
//   - Bare        Model and Version empty, no Structure — a reserved address such
//     as _raw, with nothing to validate against
type DataContract struct {
	Structure      map[string]Field
	Name           string
	Model          string
	Version        string
	Description    string
	DefaultBridges []map[string]interface{}
}

// MigrationNoticeLevel separates "this changed and you should look" from "this is
// how it was recorded".
type MigrationNoticeLevel string

const (
	NoticeInfo MigrationNoticeLevel = "info"
	NoticeWarn MigrationNoticeLevel = "warn"
)

// MigrationNotice records one decision absorb made.
//
// Notices are returned rather than logged so that absorb stays a pure function
// and its decisions can be asserted. They carry Model and Version as well as
// Contract because a dropped model version has no contract name to report with.
type MigrationNotice struct {
	Level    MigrationNoticeLevel
	Contract string
	Model    string
	Version  string
	Reason   string
	// Dropped separates "something is gone" from "something is wrong".
	//
	// Both are worth a warning, but only the first is information loss, and the
	// round-trip property is about loss alone. An orphaned contract warns loudly and
	// is kept; a duplicate address warns and is not.
	Dropped bool
}

// AbsorbConfig folds the two on-disk sections into one flat contract list.
//
// It reads both shapes: the merged form groups versions under a model label,
// while the pre-merge form keeps structures in dataModels and points at them from
// dataContracts. Order is preserved — entries first, in the order they appeared,
// then definitions taken from model versions no contract claimed.
//
// It returns no error. A parse failure caches an empty config against the new
// mtime, after which the fast path serves it with a nil error and every manager
// is told nothing is configured — silently. Refusing to load is worse than
// loading with a notice, so every ambiguity resolves deterministically instead.
func AbsorbConfig(
	models []DataModelsConfig,
	entries []DataContractYAMLEntry,
) ([]DataContract, []MigrationNotice) {
	var notices []MigrationNotice

	modelIndex, dupNotices := indexModels(models)
	notices = append(notices, dupNotices...)

	contracts := make([]DataContract, 0, len(entries))
	claimed := make(map[string]bool, len(models))
	seenNames := make(map[string]bool, len(entries))

	addNamed := func(c DataContract) {
		if c.Name != "" {
			if seenNames[c.Name] {
				notices = append(notices, MigrationNotice{
					Level:    NoticeWarn,
					Contract: c.Name,
					Model:    c.Model,
					Version:  c.Version,
					Reason: fmt.Sprintf(
						"dropped: another contract is already published at %q", c.Name),
					Dropped: true,
				})

				return
			}

			seenNames[c.Name] = true
		}

		contracts = append(contracts, c)
	}

	for _, entry := range entries {
		switch {
		case entry.LegacyModelRef != nil:
			ref := entry.LegacyModelRef

			if entry.Name == "" {
				// A pre-merge contract is an address; without one it enforces
				// nothing and there is nowhere to write it back to.
				notices = append(notices, MigrationNotice{
					Level:   NoticeWarn,
					Model:   ref.Name,
					Version: ref.Version,
					Reason:  "dropped: a data contract with no name has no address to enforce",
					Dropped: true,
				})

				continue
			}

			version, ok := modelIndex[modelKey(ref.Name, ref.Version)]
			if !ok {
				// Kept as a bare address, not dropped. The address exists and may still
				// be receiving data; dropping it here would remove its schema registry
				// subjects on the next reconcile, silently ending enforcement on a live
				// topic. As a bare address it keeps its subjects, shows up in the
				// Console as a contract with no definition, and can be deleted
				// deliberately.
				notices = append(notices, MigrationNotice{
					Level:    NoticeWarn,
					Contract: entry.Name,
					Model:    ref.Name,
					Version:  ref.Version,
					Reason: fmt.Sprintf(
						"kept as an address with no structure: it points at data model %q "+
							"version %q, which does not exist", ref.Name, ref.Version),
				})

				addNamed(DataContract{
					Name:           entry.Name,
					DefaultBridges: entry.DefaultBridges,
				})

				continue
			}

			claimed[modelKey(ref.Name, ref.Version)] = true

			addNamed(DataContract{
				Name:           entry.Name,
				Model:          ref.Name,
				Version:        ref.Version,
				Structure:      version.structure,
				Description:    version.description,
				DefaultBridges: entry.DefaultBridges,
			})

		case entry.Model == "" && len(entry.Versions) > 0:
			notices = append(notices, MigrationNotice{
				Level:   NoticeWarn,
				Reason:  "dropped: versions were given without a model to group them under",
				Dropped: true,
			})

		case len(entry.Versions) > 0:
			for _, versionKey := range sortedVersionKeys(entry.Versions) {
				version := entry.Versions[versionKey]

				if len(version.Names) == 0 && len(version.DefaultBridges) > 0 {
					// Nowhere to record it: a definition has no dataContracts entry
					// to carry it, so keeping it would break the round trip.
					notices = append(notices, MigrationNotice{
						Level:   NoticeWarn,
						Model:   entry.Model,
						Version: versionKey,
						Reason:  "default_bridges dropped: a version with no address cannot carry it",
						Dropped: true,
					})
				}

				claimed[modelKey(entry.Model, versionKey)] = true

				base := DataContract{
					Model:       entry.Model,
					Version:     versionKey,
					Structure:   version.Structure,
					Description: entry.Description,
				}

				if len(version.Names) == 0 {
					// A definition: one contract, no address.
					contracts = append(contracts, base)

					continue
				}

				// One structure published under several addresses becomes one
				// contract per address; they are separately deletable.
				for _, name := range version.Names {
					contract := base
					contract.Name = name
					contract.DefaultBridges = version.DefaultBridges
					addNamed(contract)
				}
			}

		case entry.Name != "":
			addNamed(DataContract{
				Name:           entry.Name,
				DefaultBridges: entry.DefaultBridges,
			})

		default:
			notices = append(notices, MigrationNotice{
				Level:   NoticeWarn,
				Reason:  "dropped: an entry with no name, no model and no versions describes nothing",
				Dropped: true,
			})
		}
	}

	// Every model version no contract claimed becomes a definition. Nothing
	// changes for it: it publishes no subject and is addressable by nothing,
	// which is what it did before. Giving it an address is a separate decision.
	emitted := make(map[string]bool, len(models))

	for _, model := range models {
		if model.Name == "" {
			// A structure with no lineage and no address describes nothing, and
			// cannot be written back in either shape.
			notices = append(notices, MigrationNotice{
				Level:   NoticeWarn,
				Reason:  "dropped: a data model with no name cannot be addressed or referenced",
				Dropped: true,
			})

			continue
		}

		for _, versionKey := range sortedLegacyVersionKeys(model.Versions) {
			key := modelKey(model.Name, versionKey)

			// emitted, not only claimed: two models can share a name, and emitting one
			// definition per declaration would put two contracts on one
			// (model, version). The merged shape keys versions by name, so the second
			// would silently overwrite the first on write, losing a structure.
			if claimed[key] || emitted[key] {
				continue
			}

			emitted[key] = true

			// Taken from the index rather than from this declaration, so a duplicate
			// name resolves last-wins here exactly as it does in the schema registry's
			// own map. The two agreeing is what makes the choice invisible.
			version := modelIndex[key]

			contracts = append(contracts, DataContract{
				Model:       model.Name,
				Version:     versionKey,
				Structure:   version.structure,
				Description: version.description,
			})
		}
	}

	// Nil rather than an empty slice: manager.go compares a cached config against
	// FullConfig{} with reflect.DeepEqual to mean "nothing loaded yet", and a
	// non-nil empty slice would make a genuinely empty config look loaded.
	if len(contracts) == 0 {
		return nil, notices
	}

	return contracts, notices
}

// ToLegacyConfig is the inverse: it writes the flat contracts back as the two
// pre-merge sections. This is what `downgrade-config` uses, and what consumers
// that still expect the old views are handed.
func ToLegacyConfig(contracts []DataContract) ([]DataModelsConfig, []DataContractsConfig) {
	// Allocated rather than declared so that indexing models[idx] below is provably
	// in range; the nil-analysis pass cannot see that idx always came from a preceding
	// append.
	var (
		models      = make([]DataModelsConfig, 0, len(contracts))
		modelAt     = map[string]int{}
		legacyConts = make([]DataContractsConfig, 0, len(contracts))
	)

	for _, c := range contracts {
		if c.Model != "" {
			idx, ok := modelAt[c.Model]
			if !ok {
				idx = len(models)
				modelAt[c.Model] = idx
				models = append(models, DataModelsConfig{
					Name:        c.Model,
					Description: c.Description,
					Versions:    map[string]DataModelVersion{},
				})
			}

			models[idx].Versions[c.Version] = DataModelVersion{Structure: c.Structure}
		}

		if c.Name == "" {
			continue
		}

		entry := DataContractsConfig{
			Name:           c.Name,
			DefaultBridges: c.DefaultBridges,
		}
		if c.Model != "" {
			entry.Model = &ModelRef{Name: c.Model, Version: c.Version}
		}

		legacyConts = append(legacyConts, entry)
	}

	return models, legacyConts
}

// ToYAMLEntries writes the flat contracts back as the merged section, grouped by
// model label. This is what gets marshalled to disk.
func ToYAMLEntries(contracts []DataContract) []DataContractYAMLEntry {
	// Allocated for the same reason as in ToLegacyConfig.
	var (
		entries = make([]DataContractYAMLEntry, 0, len(contracts))
		groupAt = map[string]int{}
	)

	for _, c := range contracts {
		if c.Model == "" {
			entries = append(entries, DataContractYAMLEntry{
				Name:           c.Name,
				DefaultBridges: c.DefaultBridges,
			})

			continue
		}

		idx, ok := groupAt[c.Model]
		if !ok {
			idx = len(entries)
			groupAt[c.Model] = idx
			entries = append(entries, DataContractYAMLEntry{
				Model:       c.Model,
				Description: c.Description,
				Versions:    map[string]DataContractVersion{},
			})
		}

		version := entries[idx].Versions[c.Version]
		version.Structure = c.Structure

		if c.Name != "" {
			version.Names = append(version.Names, c.Name)
			version.DefaultBridges = c.DefaultBridges
		}

		entries[idx].Versions[c.Version] = version
	}

	return entries
}

// modelVersionEntry is a model version plus the description of the model it
// belongs to, since the pre-merge shape holds one description per model.
type modelVersionEntry struct {
	structure   map[string]Field
	description string
}

func modelKey(model, version string) string { return model + "\x00" + version }

// Version keys are sorted only to make the output deterministic. Ordering of
// contracts otherwise follows the input, because determinism comes from
// preserving order rather than imposing one.
func sortedVersionKeys(versions map[string]DataContractVersion) []string {
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func sortedLegacyVersionKeys(versions map[string]DataModelVersion) []string {
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// indexModels flattens the models section for lookup. A duplicate model name
// last-wins, which is what the schema registry's own map does today; the status
// generator emits both rows, so this is the one place the two disagree and the
// merge has to pick. It picks the translator's behaviour, because that is what
// determines which schemas exist.
func indexModels(models []DataModelsConfig) (map[string]modelVersionEntry, []MigrationNotice) {
	index := make(map[string]modelVersionEntry)

	var (
		notices []MigrationNotice
		seen    = map[string]bool{}
	)

	for _, model := range models {
		if model.Name == "" {
			continue
		}

		if seen[model.Name] {
			notices = append(notices, MigrationNotice{
				Level:   NoticeWarn,
				Model:   model.Name,
				Reason:  fmt.Sprintf("data model %q is declared more than once; the last one wins", model.Name),
				Dropped: true,
			})
		}

		seen[model.Name] = true

		for versionKey, version := range model.Versions {
			index[modelKey(model.Name, versionKey)] = modelVersionEntry{
				structure:   version.Structure,
				description: model.Description,
			}
		}
	}

	return index, notices
}
