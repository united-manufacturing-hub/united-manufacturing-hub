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
	"strconv"
	"strings"
)

// maxLocationLevel bounds the level index a location mapping may define. The
// ISA-95 hierarchy has five levels; the bound keeps one config line from
// forcing a merge loop over a billion entries.
const maxLocationLevel = 1000

// canonicalLevelIndex returns the level index a location key names and
// whether the key spells the index the way strconv.Itoa does. A key like
// "01" or "+2" parses as an integer but is not canonical, so it names no
// level.
func canonicalLevelIndex(key string) (int, bool) {
	n, err := strconv.Atoi(key)
	if err != nil || n < 0 || strconv.Itoa(n) != key {
		return 0, false
	}

	return n, true
}

// MergeAgentLocation merges the agent's location with a worker's own. Map keys
// are level indexes, the canonical string form of 0, 1, 2, ..., not the named
// ISA-95 tiers of LocationLevel. The agent's value is kept on a level index
// both define, the worker's own value fills a missing or empty agent level and
// extends the map, and level indexes below the highest that neither side
// defines become "unknown". A level explicitly set to the empty string stays
// empty. The path joins levels 0..max with dots and drops trailing dots.
// Neither input map is modified. Callers normally pass maps produced by
// LocationFromAny, which rejects keys that are not canonical level indexes; a
// key that is not one is copied to the merged map unchanged and never affects
// the path.
func MergeAgentLocation(agent, own map[string]string) (map[string]string, string) {
	max := -1

	for key := range agent {
		if n, ok := canonicalLevelIndex(key); ok && n <= maxLocationLevel && n > max {
			max = n
		}
	}

	for key := range own {
		if n, ok := canonicalLevelIndex(key); ok && n <= maxLocationLevel && n > max {
			max = n
		}
	}

	merged := make(map[string]string, max+1)

	for k, v := range agent {
		merged[k] = v
	}

	for k, v := range own {
		if agentValue, exists := merged[k]; !exists || agentValue == "" {
			merged[k] = v
		}
	}

	for i := 0; i <= max; i++ {
		key := strconv.Itoa(i)

		if _, exists := merged[key]; !exists {
			merged[key] = "unknown"
		}
	}

	parts := make([]string, 0, max+1)

	for i := 0; i <= max; i++ {
		key := strconv.Itoa(i)

		if v, exists := merged[key]; exists {
			parts = append(parts, v)
		}
	}

	return merged, strings.TrimRight(strings.Join(parts, "."), ".")
}

// LocationFromAny converts an untyped location mapping into a map of level
// index to value. It accepts map[string]string, map[string]any and
// map[any]any, the forms YAML and JSON decoding produce, and every key is
// taken through fmt.Sprint, so the integer key 1 becomes "1". nil gives an
// empty map. Every key must be a non-negative canonical integer no greater
// than maxLocationLevel, no two keys may spell the same level index, and every
// value must be a string or a number. Anything else is a *ConfigError on the
// location field.
func LocationFromAny(raw any) (map[string]string, error) {
	var entries map[any]any

	switch m := raw.(type) {
	case nil:
		return map[string]string{}, nil
	case map[string]string:
		entries = make(map[any]any, len(m))
		for k, v := range m {
			entries[k] = v
		}
	case map[string]any:
		entries = make(map[any]any, len(m))
		for k, v := range m {
			entries[k] = v
		}
	case map[any]any:
		entries = m
	default:
		return nil, &ConfigError{Field: "location", Err: fmt.Errorf("location must be a mapping, got %T", raw)}
	}

	out := make(map[string]string, len(entries))

	for rawKey, rawValue := range entries {
		key := fmt.Sprint(rawKey)

		n, canonical := canonicalLevelIndex(key)
		if !canonical {
			return nil, &ConfigError{Field: "location", Err: fmt.Errorf("level key %q is not a non-negative canonical integer", key)}
		}

		if n > maxLocationLevel {
			return nil, &ConfigError{Field: "location", Err: fmt.Errorf("level %d exceeds the maximum of %d", n, maxLocationLevel)}
		}

		if _, exists := out[key]; exists {
			return nil, &ConfigError{Field: "location", Err: fmt.Errorf("duplicate level key %q", key)}
		}

		switch value := rawValue.(type) {
		case string:
			out[key] = value
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			out[key] = fmt.Sprint(value)
		default:
			return nil, &ConfigError{Field: "location", Err: fmt.Errorf("level %q: value must be a string or a number", key)}
		}
	}

	return out, nil
}
