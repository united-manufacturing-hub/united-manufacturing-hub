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
	"bytes"
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ErrDowngradeNotLossless is returned when the contracts cannot be expressed in the
// pre-merge shape. Writing a partial downgrade would be worse than refusing: the
// older release would read it, find fewer contracts than it should, and tear down
// the bridges belonging to the missing ones without reporting anything.
var ErrDowngradeNotLossless = errors.New(
	"this config cannot be converted to the pre-merge format without losing contracts")

// downgradeIndent matches the two-space style of every config in examples/ and of
// hand-written files. yaml.Marshal would use four.
const downgradeIndent = 2

// DowngradeConfigYAML rewrites a config in the pre-merge shape: structures back in
// dataModels, addresses back in dataContracts pointing at them.
//
// This must be run before moving an instance to a release from before the merge.
// Such a release decodes `model: pump` as a mapping, fails, and caches an empty
// config against a matching mtime -- after which every manager is told nothing is
// configured, with a nil error. The fix for that swallowed error cannot help here,
// because it would have to be in the older binary doing the reading.
//
// Only the two contract sections are touched. The rest of the document is edited in
// place as YAML nodes rather than round-tripped through FullConfig, which matters
// more than it sounds:
//
//   - comments and anchors survive, and config.yaml is hand-edited in practice
//   - fields the struct defaults are not silently rewritten. Re-marshalling a
//     FullConfig writes every field, so a file that omits
//     agent.enableResourceLimitBlocking would come back with it set to false --
//     flipping a flag that defaults to true, during a rollback, in a file nobody is
//     reading closely.
func DowngradeConfigYAML(ctx context.Context, data []byte) ([]byte, error) {
	config, _, err := ParseConfigWithNotices(data, ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	models, legacy := ToLegacyConfig(config.Contracts)
	entries := LegacyEntries(legacy)

	// Verified against what the older release will actually read. Absorbing the
	// pre-merge pair has to reproduce the contracts we started from, or something was
	// lost in the projection itself.
	roundTripped, notices := AbsorbConfig(models, entries)
	if FirstDrop(notices) != nil || !ContractsEqual(config.Contracts, roundTripped) {
		return nil, ErrDowngradeNotLossless
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse config as YAML: %w", err)
	}

	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("config is not a YAML mapping")
	}

	document := root.Content[0]

	if err := setMappingKey(document, "dataModels", models); err != nil {
		return nil, err
	}

	if err := setMappingKey(document, "dataContracts", entries); err != nil {
		return nil, err
	}

	var out bytes.Buffer

	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(downgradeIndent)

	if err := encoder.Encode(&root); err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	return out.Bytes(), nil
}

// setMappingKey replaces a key's value in place, appends it if absent, and removes
// it if the value is empty.
//
// In place, because position carries meaning to whoever reads the file next: moving
// dataModels to the bottom during a rollback makes the diff harder to check at
// exactly the wrong moment.
func setMappingKey(document *yaml.Node, key string, value interface{}) error {
	empty := isEmptySequence(value)

	for i := 0; i+1 < len(document.Content); i += 2 {
		if document.Content[i].Value != key {
			continue
		}

		if empty {
			document.Content = append(document.Content[:i], document.Content[i+2:]...)

			return nil
		}

		encoded := &yaml.Node{}
		if err := encoded.Encode(value); err != nil {
			return fmt.Errorf("encoding %s: %w", key, err)
		}

		// The key node is left alone so its comments stay attached.
		document.Content[i+1] = encoded

		return nil
	}

	if empty {
		return nil
	}

	encoded := &yaml.Node{}
	if err := encoded.Encode(value); err != nil {
		return fmt.Errorf("encoding %s: %w", key, err)
	}

	document.Content = append(document.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		encoded)

	return nil
}

func isEmptySequence(value interface{}) bool {
	switch typed := value.(type) {
	case []DataModelsConfig:
		return len(typed) == 0
	case []DataContractYAMLEntry:
		return len(typed) == 0
	default:
		return false
	}
}
