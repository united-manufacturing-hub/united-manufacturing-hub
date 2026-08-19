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

package benthosserviceconfig

import (
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
)

// useCanonicalizeFast gates the walk in canonicalize_walk.go behind
// USE_CANONICALIZE_FAST. Opt-out and on by default: normalizeValue is a
// hand-written reimplementation of what yaml.Marshal/Unmarshal do (see the SAFETY
// note there), so a shape it gets wrong would make two equal configs compare
// unequal on every tick. Setting USE_CANONICALIZE_FAST=false turns it off in
// place, without a rollback, if that ever happens in production; every section
// then takes the YAML round-trip that predates it.
var useCanonicalizeFast = canonicalizeFastEnabled()

// canonicalizeFastEnabled reads the gate. Separate from the variable so a test can
// assert what an unset or explicitly disabled USE_CANONICALIZE_FAST resolves to;
// the variable itself is fixed at process start and carries whatever the ambient
// environment said.
func canonicalizeFastEnabled() bool {
	enabled, _ := env.GetAsBool("USE_CANONICALIZE_FAST", false, true)

	return enabled
}

// canonicalize rewrites the free-form config maps into the types they take once
// the config has been written to benthos.yaml and read back, so a config built in
// Go compares equal to the same config parsed from disk. Without it a purely
// representational difference - a Go []string against the []interface{} yaml
// decodes - looks semantic and the config is re-applied on every tick.
//
// MetricsPort and DebugLevel are left alone: scalars need no canonicalization, and
// the document spells them as an http address and a log level.
//
// Each section is canonicalized on its own, so a value the walk declines costs the
// round-trip for its own section and no others. The result shares unreplaced maps
// with cfg, which callers must not mutate.
func canonicalize(cfg BenthosServiceConfig) BenthosServiceConfig {
	cfg.Input = canonicalizeMap(cfg.Input)
	cfg.Output = canonicalizeMap(cfg.Output)
	cfg.Pipeline = canonicalizeMap(cfg.Pipeline)
	cfg.Buffer = canonicalizeMap(cfg.Buffer)
	cfg.CacheResources = canonicalizeResources(cfg.CacheResources)
	cfg.RateLimitResources = canonicalizeResources(cfg.RateLimitResources)

	return cfg
}

// canonicalizeMap returns m in the shape it takes after a YAML round-trip. A nil
// or empty map is returned unchanged, since the comparator wants the normalizer's
// empty map rather than the sequence an empty section renders as.
func canonicalizeMap(m map[string]interface{}) map[string]interface{} {
	if len(m) == 0 {
		return m
	}

	out, err := roundTrip(m)
	if err != nil {
		return m
	}

	result, ok := out.(map[string]interface{})
	if !ok {
		return m
	}

	return result
}

// canonicalizeResources returns the resource slice in the shape it takes after a
// YAML round-trip. A nil or empty slice is returned unchanged.
func canonicalizeResources(s []map[string]interface{}) []map[string]interface{} {
	if len(s) == 0 {
		return s
	}

	out, err := roundTrip(s)
	if err != nil {
		return s
	}

	list, ok := out.([]interface{})
	if !ok {
		return s
	}

	result := make([]map[string]interface{}, 0, len(list))

	for _, item := range list {
		entry, ok := item.(map[string]interface{})
		if !ok {
			return s
		}

		result = append(result, entry)
	}

	return result
}

// roundTrip marshals v to YAML and unmarshals it back into a generic value.
//
// normalizeValue is tried first when useCanonicalizeFast is set: it walks the value
// instead of serialising it, and reports ok=false for anything it cannot reproduce
// exactly, in which case we fall through to the round-trip below. See
// canonicalize_walk.go for what it declines.
//
// On a marshal or unmarshal error the caller keeps the original value, so
// canonicalization can never make two equal configs look different.
func roundTrip(v interface{}) (interface{}, error) {
	if useCanonicalizeFast {
		out, ok, unsupported := normalizeValue(v)
		if ok {
			return out, nil
		}

		reportFallback(unsupported)
	}

	encoded, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	var decoded interface{}
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}

	return decoded, nil
}
