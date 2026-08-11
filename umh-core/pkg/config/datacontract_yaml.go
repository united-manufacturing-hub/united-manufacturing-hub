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

	"gopkg.in/yaml.v3"
)

// DataContractVersion is one version inside a grouped dataContracts entry.
//
// Names are the addresses this version is published under in UNS topics.
// It accepts a scalar or a list, and marshals back as whichever it needs:
//
//	name: _pump_v1                  # the ordinary case
//	name: [_pump_v1, _pumpalt_v1]   # one structure, several addresses
//
// Empty means a definition — a structure other contracts can reference but that
// nothing publishes to. A list is necessary rather than a nicety: two contracts
// can share one model version, and with a single name the second address would be
// silently dropped, which stops validating a topic that is still receiving data.
type DataContractVersion struct {
	Structure      map[string]Field         `yaml:"structure"`
	Names          []string                 `yaml:"name,omitempty"`
	DefaultBridges []map[string]interface{} `yaml:"default_bridges,omitempty"`
}

// MarshalYAML writes a single address as a scalar so the ordinary case stays
// readable, and several as a list.
//
// Built as a node rather than a map or a struct because the key order is
// deliberate -- the address first, the structure last, since the structure is the
// long part -- and neither a map (yaml.v3 sorts its keys) nor a struct (the
// alignment linter reorders its fields) can express that.
func (v DataContractVersion) MarshalYAML() (interface{}, error) {
	out := newMappingNode()

	switch len(v.Names) {
	case 0:
	case 1:
		if err := out.add("name", v.Names[0]); err != nil {
			return nil, err
		}
	default:
		if err := out.add("name", v.Names); err != nil {
			return nil, err
		}
	}

	if len(v.DefaultBridges) > 0 {
		if err := out.add("default_bridges", v.DefaultBridges); err != nil {
			return nil, err
		}
	}

	if err := out.add("structure", v.Structure); err != nil {
		return nil, err
	}

	return out.node, nil
}

// orderedMapping builds a YAML mapping whose key order is what it was written in.
type orderedMapping struct {
	node *yaml.Node
}

func newMappingNode() orderedMapping {
	return orderedMapping{node: &yaml.Node{Kind: yaml.MappingNode}}
}

func (m orderedMapping) add(key string, value interface{}) error {
	encoded := &yaml.Node{}
	if err := encoded.Encode(value); err != nil {
		return fmt.Errorf("encoding %s: %w", key, err)
	}

	m.node.Content = append(m.node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		encoded)

	return nil
}

// decodeNames accepts a scalar address or a list of them.
func decodeNames(node *yaml.Node, into *[]string) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var one string
		if err := node.Decode(&one); err != nil {
			return fmt.Errorf("line %d: name: %w", node.Line, err)
		}

		*into = []string{one}

		return nil
	case yaml.SequenceNode:
		return node.Decode(into)
	default:
		return fmt.Errorf("line %d: name must be an address or a list of addresses", node.Line)
	}
}

// DataContractYAMLEntry is one entry of the dataContracts section.
//
// The section holds three forms and this type is the union of them:
//
//   - grouped: model + versions, each version optionally carrying its address
//   - bare: name alone, with no model and no structure (_raw)
//   - legacy: name plus a model *mapping* pointing at a dataModels version
//
// The legacy form is what every config in the field looks like today, so it has
// to decode. `model` is a scalar in the grouped form and a mapping in the legacy
// one, which is what makes struct tags insufficient.
//
// AbsorbConfig turns these into flat DataContract values; nothing outside this
// package should read this type.
type DataContractYAMLEntry struct {
	Versions map[string]DataContractVersion `yaml:"versions,omitempty"`
	// LegacyModelRef is set only by the pre-merge form. Never written back.
	LegacyModelRef *ModelRef `yaml:"-"`
	Model          string    `yaml:"model,omitempty"`
	Name           string    `yaml:"name,omitempty"`
	Description    string    `yaml:"description,omitempty"`
	// DefaultBridges at entry level belongs to the legacy form; the grouped form
	// carries it per version, which is where it actually sits.
	DefaultBridges []map[string]interface{} `yaml:"default_bridges,omitempty"`
}

// dataContractEntryOut is the marshalling form of an entry.
//
// A struct rather than a map so the field order is stable and readable -- yaml.v3
// sorts map keys, which would put default_bridges above the model it belongs to.
// Model is typed loosely because it is a label in the merged shape and a
// {name, version} mapping in the pre-merge one, and downgrade-config has to emit
// the latter.
//
// MarshalYAML writes whichever of the three forms this entry holds.
//
// LegacyModelRef would otherwise be dropped: it is tagged `yaml:"-"` so a normal
// write can never emit the pre-merge shape by accident. downgrade-config is the one
// caller that wants it, so it is spelled out here rather than made reachable through
// a struct tag.
func (e DataContractYAMLEntry) MarshalYAML() (interface{}, error) {
	out := newMappingNode()

	if e.Name != "" {
		if err := out.add("name", e.Name); err != nil {
			return nil, err
		}
	}

	// A label in the merged shape, a {name, version} mapping in the pre-merge one.
	switch {
	case e.LegacyModelRef != nil:
		if err := out.add("model", e.LegacyModelRef); err != nil {
			return nil, err
		}
	case e.Model != "":
		if err := out.add("model", e.Model); err != nil {
			return nil, err
		}
	}

	if e.Description != "" {
		if err := out.add("description", e.Description); err != nil {
			return nil, err
		}
	}

	if len(e.DefaultBridges) > 0 {
		if err := out.add("default_bridges", e.DefaultBridges); err != nil {
			return nil, err
		}
	}

	// Last, because it is the long part.
	if len(e.Versions) > 0 {
		if err := out.add("versions", e.Versions); err != nil {
			return nil, err
		}
	}

	return out.node, nil
}

// entryKeys and versionKeys are the only keys accepted on each mapping.
//
// They exist because a custom UnmarshalYAML silently disables the decoder's
// KnownFields setting: yaml.Node.Decode constructs its own decoder with
// knownFields false, so strictness cannot be inherited. Without checking keys
// here, a typo'd `structrue:` would be accepted, the contract would lose its
// structure, and its schema registry subjects would then be deleted as unknown.
var (
	entryKeys = map[string]bool{
		"versions": true, "model": true, "name": true,
		"description": true, "default_bridges": true,
	}
	versionKeys = map[string]bool{
		"structure": true, "name": true, "default_bridges": true,
	}
)

// mappingPairs returns the key/value node pairs of a mapping, rejecting anything
// that is not a mapping.
func mappingPairs(node *yaml.Node, what string) ([][2]*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: %s must be a mapping", node.Line, what)
	}

	pairs := make([][2]*yaml.Node, 0, len(node.Content)/2)

	for i := 0; i+1 < len(node.Content); i += 2 {
		pairs = append(pairs, [2]*yaml.Node{node.Content[i], node.Content[i+1]})
	}

	return pairs, nil
}

// UnmarshalYAML decodes any of the three entry forms.
func (e *DataContractYAMLEntry) UnmarshalYAML(node *yaml.Node) error {
	pairs, err := mappingPairs(node, "a dataContracts entry")
	if err != nil {
		return err
	}

	for _, pair := range pairs {
		key, value := pair[0].Value, pair[1]

		if !entryKeys[key] {
			return fmt.Errorf("line %d: unknown field %q in a dataContracts entry", pair[0].Line, key)
		}

		switch key {
		case "model":
			// A scalar is the grouping label; a mapping is the pre-merge pointer.
			switch value.Kind {
			case yaml.ScalarNode:
				if err := value.Decode(&e.Model); err != nil {
					return fmt.Errorf("line %d: model: %w", value.Line, err)
				}
			case yaml.MappingNode:
				ref := &ModelRef{}
				if err := value.Decode(ref); err != nil {
					return fmt.Errorf("line %d: model: %w", value.Line, err)
				}

				e.LegacyModelRef = ref
			default:
				return fmt.Errorf(
					"line %d: model must be a name or a {name, version} reference", value.Line)
			}
		case "versions":
			versions, err := decodeVersions(value)
			if err != nil {
				return err
			}

			e.Versions = versions
		case "name":
			if err := value.Decode(&e.Name); err != nil {
				return fmt.Errorf("line %d: name: %w", value.Line, err)
			}
		case "description":
			if err := value.Decode(&e.Description); err != nil {
				return fmt.Errorf("line %d: description: %w", value.Line, err)
			}
		case "default_bridges":
			if err := value.Decode(&e.DefaultBridges); err != nil {
				return fmt.Errorf("line %d: default_bridges: %w", value.Line, err)
			}
		}
	}

	if e.LegacyModelRef != nil && len(e.Versions) > 0 {
		return fmt.Errorf(
			"line %d: a dataContracts entry has either a model reference or versions, not both",
			node.Line)
	}

	return nil
}

// decodeVersions reads the versions map, checking each version's keys for the
// same reason entryKeys exists.
func decodeVersions(node *yaml.Node) (map[string]DataContractVersion, error) {
	pairs, err := mappingPairs(node, "versions")
	if err != nil {
		return nil, err
	}

	versions := make(map[string]DataContractVersion, len(pairs))

	for _, pair := range pairs {
		versionKey, body := pair[0].Value, pair[1]

		bodyPairs, err := mappingPairs(body, "version "+versionKey)
		if err != nil {
			return nil, err
		}

		var version DataContractVersion

		for _, field := range bodyPairs {
			key, value := field[0].Value, field[1]

			if !versionKeys[key] {
				return nil, fmt.Errorf(
					"line %d: unknown field %q in version %q", field[0].Line, key, versionKey)
			}

			switch key {
			case "structure":
				// A structure's own keys are field names, so it stays permissive:
				// an unrecognised key there is a subfield, not a typo.
				if err := value.Decode(&version.Structure); err != nil {
					return nil, fmt.Errorf("line %d: structure: %w", value.Line, err)
				}
			case "name":
				if err := decodeNames(value, &version.Names); err != nil {
					return nil, err
				}
			case "default_bridges":
				if err := value.Decode(&version.DefaultBridges); err != nil {
					return nil, fmt.Errorf("line %d: default_bridges: %w", value.Line, err)
				}
			}
		}

		versions[versionKey] = version
	}

	return versions, nil
}
