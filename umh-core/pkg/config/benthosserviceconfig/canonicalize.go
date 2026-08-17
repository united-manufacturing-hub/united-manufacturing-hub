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

// canonicalize rewrites the free-form config sections into the concrete types they
// take once the config has been written to benthos.yaml and read back, so a config
// built in Go compares equal to the same config parsed from disk. Without it a
// difference that is purely representational - a Go []string against the
// []interface{} yaml decodes - registers as a semantic divergence and the config is
// re-applied forever.
//
// It renders the whole document through the same generator that writes
// benthos.yaml and parses the result back. Going through the writer is what keeps it
// honest: the file is the definition of what the observed config will look like, so
// there is no second model of yaml's emitter to keep in step with yaml.v3. An
// earlier version reproduced the emitter's decisions in Go instead and needed a
// table of the value shapes it had to refuse, which grew every time a shape it got
// wrong turned up. Rendering the document also serializes a config once rather than
// once per section.
//
// MetricsPort and DebugLevel are kept as they are. They are scalars that need no
// canonicalization, and the document spells them as an address and a log level
// rather than as themselves.
//
// Canonicalization is best-effort: on a render or parse error the config is
// returned unchanged, so it can never make two equal configs look different.
//
// The returned config shares the maps it did not replace with cfg. Callers must not
// mutate them.
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
// output as an empty sequence rather than a map, and the comparator relies on the
// normalizer's empty map rather than on what the document says.
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
// A cache keyed on the rendered text measured 1.7x on this path and nothing at all
// on the path that used to be slow: ConfigsEqual only gets here for a config built
// in Go, and those are small. It is not worth holding a rendered document and its
// parse tree per component for the lifetime of the process.
func parseCanonicalDoc(text string) (map[string]interface{}, bool) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, false
	}

	return doc, true
}
