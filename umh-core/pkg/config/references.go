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
	"strings"

	"gopkg.in/yaml.v3"
)

// ReferenceKind says how a reference was found, which matters more than that one
// was: "a stream processor declares this model" is a fact, whereas "a bridge's
// config contains this string" is a guess that happens to be right nearly always.
// A refusal has to tell the user which it is, or they cannot judge it.
type ReferenceKind string

const (
	// ReferenceKindStreamProcessor is a declared model binding. Exact.
	ReferenceKindStreamProcessor ReferenceKind = "streamProcessor"
	// ReferenceKindRefModel is a _refModel pointer from another contract's
	// structure. Exact, and transitive.
	ReferenceKindRefModel ReferenceKind = "refModel"
	// ReferenceKindBridge is a delimiter-anchored string match in a bridge's
	// config. Inexact by nature: bridges name topics as text, so this is the best
	// available evidence rather than proof.
	ReferenceKindBridge ReferenceKind = "bridge"
)

// Reference is one thing standing in the way of a deletion.
type Reference struct {
	Kind   ReferenceKind
	Name   string // the referring object
	Detail string // how the reference was found, in the user's terms
}

// String renders a reference for an error message the user will read in the
// Console, so it names the referrer and the evidence together.
func (r Reference) String() string {
	return fmt.Sprintf("%s %q (%s)", r.Kind, r.Name, r.Detail)
}

// FindModelReferences reports everything that depends on a contract group.
//
// Deleting a group means deleting the structure, so both the declared bindings and
// the structural pointers matter. Addresses are checked too: a bridge publishing to
// one of the group's addresses is broken by the deletion just as surely.
//
// Known gap: a bridge that assembles its topic at runtime -- in a mapping
// expression, or from a variable -- cannot be found by any static search, so a
// group with only such a referrer will delete cleanly and break it. Detecting that
// needs live topic activity, which this function has no access to.
func FindModelReferences(cfg FullConfig, label string) []Reference {
	var refs []Reference

	refs = append(refs, streamProcessorRefs(cfg, label)...)
	refs = append(refs, refModelRefs(cfg, label)...)

	for _, contract := range cfg.Contracts {
		if contract.Model == label && contract.Name != "" {
			refs = append(refs, bridgeRefs(cfg, contract.Name)...)
		}
	}

	return dedupeRefs(refs)
}

// FindAddressReferences reports everything that depends on a single address.
//
// Only bridges can: a stream processor names its model, and _refModel names a
// model, so neither is affected by an address going away while its structure
// stays.
func FindAddressReferences(cfg FullConfig, address string) []Reference {
	return dedupeRefs(bridgeRefs(cfg, address))
}

// streamProcessorRefs matches on the stored (name, version) pair and on the
// composed address.
//
// Both, because they can disagree. Benthos composes the topic it writes to from
// the stored pair, so the pair is what actually binds; but a config can also name
// the contract directly, and if someone renamed the address without updating the
// pair, only one of the two matches. Missing either way loses the reference.
func streamProcessorRefs(cfg FullConfig, label string) []Reference {
	var refs []Reference

	for _, sp := range cfg.StreamProcessor {
		model := sp.StreamProcessorServiceConfig.Config.Model

		if model.Name == label {
			refs = append(refs, Reference{
				Kind: ReferenceKindStreamProcessor,
				Name: sp.Name,
				Detail: fmt.Sprintf("declares model %q version %q",
					model.Name, model.Version),
			})

			continue
		}

		// The composed form, for a config that names the contract rather than the
		// pair behind it.
		composed := "_" + label + "_"
		if model.Name != "" && strings.HasPrefix(model.Name, composed) {
			refs = append(refs, Reference{
				Kind:   ReferenceKindStreamProcessor,
				Name:   sp.Name,
				Detail: fmt.Sprintf("declares model %q, which addresses %q", model.Name, label),
			})
		}
	}

	return refs
}

// refModelRefs finds contracts whose structure points at this model, following
// pointers through nested models so an indirect dependency still counts.
func refModelRefs(cfg FullConfig, label string) []Reference {
	structures := make(map[string][]map[string]Field)

	for _, contract := range cfg.Contracts {
		if contract.Model != "" {
			structures[contract.Model] = append(structures[contract.Model], contract.Structure)
		}
	}

	var refs []Reference

	for _, contract := range cfg.Contracts {
		if contract.Model == label || contract.Model == "" {
			continue
		}

		if path := findRefModel(contract.Structure, label, structures, map[string]bool{}, ""); path != "" {
			refs = append(refs, Reference{
				Kind:   ReferenceKindRefModel,
				Name:   contract.Model,
				Detail: fmt.Sprintf("field %s references it via _refModel", path),
			})
		}
	}

	return refs
}

// findRefModel walks a structure for a _refModel pointing at target, returning the
// field path that reaches it or "" if none does. visited breaks reference cycles,
// which the validator permits to exist in a config even though it rejects them.
func findRefModel(
	structure map[string]Field,
	target string,
	structures map[string][]map[string]Field,
	visited map[string]bool,
	path string,
) string {
	for fieldName, field := range structure {
		here := fieldName
		if path != "" {
			here = path + "." + fieldName
		}

		if field.ModelRef != nil {
			if field.ModelRef.Name == target {
				return here
			}

			if !visited[field.ModelRef.Name] {
				visited[field.ModelRef.Name] = true

				for _, nested := range structures[field.ModelRef.Name] {
					if found := findRefModel(nested, target, structures, visited, here); found != "" {
						return found
					}
				}
			}
		}

		if found := findRefModel(field.Subfields, target, structures, visited, here); found != "" {
			return found
		}
	}

	return ""
}

// bridgeRefs finds bridges whose config mentions the address.
//
// The search is over the marshalled config rather than specific fields because a
// bridge names topics in free-form benthos YAML -- there is no field to read. It is
// delimiter-anchored so that _pump_v1 does not match inside _pump_v10, which is the
// failure that would make the guard refuse deletions for no reason and get it
// switched off.
func bridgeRefs(cfg FullConfig, address string) []Reference {
	var refs []Reference

	for _, pc := range cfg.ProtocolConverter {
		if mentions(pc.ProtocolConverterServiceConfig, address) {
			refs = append(refs, Reference{
				Kind:   ReferenceKindBridge,
				Name:   pc.Name,
				Detail: fmt.Sprintf("its configuration contains the address %q", address),
			})
		}
	}

	for _, df := range cfg.DataFlow {
		if mentions(df.DataFlowComponentServiceConfig, address) {
			refs = append(refs, Reference{
				Kind:   ReferenceKindBridge,
				Name:   df.Name,
				Detail: fmt.Sprintf("its configuration contains the address %q", address),
			})
		}
	}

	return refs
}

// mentions marshals a config and looks for the token, anchored at both ends.
func mentions(section interface{}, token string) bool {
	if token == "" {
		return false
	}

	data, err := yaml.Marshal(section)
	if err != nil {
		// Unmarshallable config cannot be searched. Reporting no reference is the
		// wrong-but-quiet answer and reporting one would block every deletion, so
		// this follows the rest of the function: absence of evidence.
		return false
	}

	return containsToken(string(data), token)
}

// containsToken reports whether token appears in text bounded by something that
// cannot be part of a topic segment.
func containsToken(text, token string) bool {
	// An empty token matches at every offset, which walks the scan past the end of
	// the string. A contract with no address cannot be referenced by name anyway.
	if token == "" {
		return false
	}

	// Terminating: offset only ever advances, and once it passes the last possible
	// start Index returns -1, since token is non-empty.
	for offset := 0; ; {
		index := strings.Index(text[offset:], token)
		if index < 0 {
			return false
		}

		start := offset + index
		end := start + len(token)

		beforeOK := start == 0 || !isTopicRune(rune(text[start-1]))
		afterOK := end == len(text) || !isTopicRune(rune(text[end]))

		if beforeOK && afterOK {
			return true
		}

		offset = start + 1
	}
}

// isTopicRune reports whether a byte can be part of a UNS topic segment. Underscore
// counts, which is what makes _pump_v1 fail to match inside _pump_v10.
func isTopicRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '_', r == '-':
		return true
	default:
		return false
	}
}

func dedupeRefs(refs []Reference) []Reference {
	if len(refs) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(refs))
	out := make([]Reference, 0, len(refs))

	for _, ref := range refs {
		key := string(ref.Kind) + "\x00" + ref.Name + "\x00" + ref.Detail
		if seen[key] {
			continue
		}

		seen[key] = true

		out = append(out, ref)
	}

	return out
}

// describeReferences renders references for a refusal message, naming at most a few
// so the message stays readable.
func describeReferences(refs []Reference) string {
	const maxNamed = 3

	parts := make([]string, 0, maxNamed)
	for i, ref := range refs {
		if i == maxNamed {
			parts = append(parts, fmt.Sprintf("and %d more", len(refs)-maxNamed))

			break
		}

		parts = append(parts, ref.String())
	}

	return strings.Join(parts, ", ")
}
