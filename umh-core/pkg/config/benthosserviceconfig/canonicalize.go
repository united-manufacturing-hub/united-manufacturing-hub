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
)

// canonicalize rewrites the free-form sections into the types they take once the
// config has been written to benthos.yaml and read back, so a config built in Go
// compares equal to the same config parsed from disk. Without it a purely
// representational difference - a Go []string against the []interface{} yaml decodes
// - looks semantic and the config is re-applied on every tick.
//
// The whole document is rendered with the generator that writes benthos.yaml and
// parsed back, which leaves the file as the single definition of the observed shape.
// An earlier version reproduced yaml.v3's emitter decisions in Go instead and needed
// an ever-growing table of value shapes to refuse.
//
// MetricsPort and DebugLevel are left alone: scalars need no canonicalization, and
// the document spells them as an http address and a log level. On a render or parse
// error cfg is returned unchanged, so canonicalization can never make two equal
// configs look different. The result shares unreplaced maps with cfg, which callers
// must not mutate.
func canonicalize(cfg BenthosServiceConfig) BenthosServiceConfig {
	text, err := defaultGenerator.RenderConfig(cfg)
	if err != nil {
		return cfg
	}

	doc, ok := parseCanonicalDoc(text)
	if !ok {
		return cfg
	}

	cfg.Input = canonicalSection(doc, "input", cfg.Input)
	cfg.Output = canonicalSection(doc, "output", cfg.Output)
	cfg.Pipeline = canonicalSection(doc, "pipeline", cfg.Pipeline)
	cfg.Buffer = canonicalSection(doc, "buffer", cfg.Buffer)
	cfg.CacheResources = canonicalResources(doc, "cache_resources", cfg.CacheResources)
	cfg.RateLimitResources = canonicalResources(doc, "rate_limit_resources", cfg.RateLimitResources)

	return cfg
}

// canonicalSection returns the named section as the rendered document parsed it, or
// original when the document does not carry it as a map.
//
// An empty section is returned untouched: the generator renders an empty input or
// output as a sequence rather than a map, and the comparator wants the normalizer's
// empty map instead.
func canonicalSection(doc map[string]interface{}, key string, original map[string]interface{}) map[string]interface{} {
	if len(original) == 0 {
		return original
	}

	section, ok := doc[key].(map[string]interface{})
	if !ok {
		return original
	}

	return section
}

// canonicalResources returns the named resource list as the rendered document
// parsed it, or original when the document does not carry it as a list of maps.
func canonicalResources(doc map[string]interface{}, key string, original []map[string]interface{}) []map[string]interface{} {
	if len(original) == 0 {
		return original
	}

	list, ok := doc[key].([]interface{})
	if !ok {
		return original
	}

	result := make([]map[string]interface{}, 0, len(list))

	for _, item := range list {
		entry, ok := item.(map[string]interface{})
		if !ok {
			return original
		}

		result = append(result, entry)
	}

	return result
}

// parseCanonicalDoc parses a rendered benthos document.
//
// Deliberately uncached: only Go-built configs reach here and they are small, so a
// cache keyed on the rendered text measured 1.7x on this path and nothing on the
// path that used to be slow - not worth a document and parse tree held per component
// for the lifetime of the process.
func parseCanonicalDoc(text string) (map[string]interface{}, bool) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, false
	}

	return doc, true
}
