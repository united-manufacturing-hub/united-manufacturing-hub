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

// useCanonicalizeFast gates canonicalizeFast behind USE_CANONICALIZE_FAST.
// Opt-out and on by default: canonicalizeFast is a hand-written reimplementation
// of what yaml.Marshal/Unmarshal do (see the SAFETY note on normalizeValue in
// canonicalize_fast.go), so a shape it gets wrong would make two equal configs
// compare unequal on every tick. Flipping this env var lets it be disabled
// instantly, without a rollback, if that ever happens in production.
var useCanonicalizeFast, _ = env.GetAsBool("USE_CANONICALIZE_FAST", false, true)

// canonicalize rewrites the free-form sections into the types they take once the
// config has been written to benthos.yaml and read back, so a config built in Go
// compares equal to the same config parsed from disk. Without it a purely
// representational difference - a Go []string against the []interface{} yaml decodes
// - looks semantic and the config is re-applied on every tick.
//
// canonicalizeFast is tried first when useCanonicalizeFast is set: it walks each
// section instead of serialising it, and reports ok=false for anything it cannot
// reproduce exactly, in which case we fall through to canonicalizeSlow. See
// canonicalize_fast.go for what it declines.
//
// MetricsPort and DebugLevel are left alone in both paths: scalars need no
// canonicalization, and the document spells them as an http address and a log
// level. The result shares unreplaced maps with cfg, which callers must not mutate.
func canonicalize(cfg BenthosServiceConfig) BenthosServiceConfig {
	if useCanonicalizeFast {
		if fast, ok := canonicalizeFast(cfg); ok {
			return fast
		}
	}

	return canonicalizeSlow(cfg)
}

// canonicalizeFast reproduces canonicalizeSlow's result for every section by
// walking the value in Go instead of rendering and reparsing the whole document.
// A single section that normalizeValue cannot reproduce exactly - see
// canonicalize_fast.go - fails the whole call, since canonicalizeSlow's single
// render already computes every section at once and gives no reason to mix the
// two paths within one call.
func canonicalizeFast(cfg BenthosServiceConfig) (BenthosServiceConfig, bool) {
	input, ok := fastSection(cfg.Input)
	if !ok {
		return cfg, false
	}

	output, ok := fastSection(cfg.Output)
	if !ok {
		return cfg, false
	}

	pipeline, ok := fastSection(cfg.Pipeline)
	if !ok {
		return cfg, false
	}

	buffer, ok := fastSection(cfg.Buffer)
	if !ok {
		return cfg, false
	}

	cacheResources, ok := fastResources(cfg.CacheResources)
	if !ok {
		return cfg, false
	}

	rateLimitResources, ok := fastResources(cfg.RateLimitResources)
	if !ok {
		return cfg, false
	}

	cfg.Input = input
	cfg.Output = output
	cfg.Pipeline = pipeline
	cfg.Buffer = buffer
	cfg.CacheResources = cacheResources
	cfg.RateLimitResources = rateLimitResources

	return cfg, true
}

// fastSection runs normalizeValue over a section map, matching canonicalSection's
// rule that an empty section is returned untouched rather than walked.
func fastSection(original map[string]interface{}) (map[string]interface{}, bool) {
	if len(original) == 0 {
		return original, true
	}

	out, ok, unsupported := normalizeValue(original)
	if !ok {
		reportFallback(unsupported)

		return nil, false
	}

	result, ok := out.(map[string]interface{})
	if !ok {
		return nil, false
	}

	return result, true
}

// fastResources runs normalizeValue over a resource list, matching
// canonicalResources's rule that an empty list is returned untouched.
func fastResources(original []map[string]interface{}) ([]map[string]interface{}, bool) {
	if len(original) == 0 {
		return original, true
	}

	out, ok, unsupported := normalizeValue(original)
	if !ok {
		reportFallback(unsupported)

		return nil, false
	}

	list, ok := out.([]interface{})
	if !ok {
		return nil, false
	}

	result := make([]map[string]interface{}, 0, len(list))

	for _, item := range list {
		entry, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}

		result = append(result, entry)
	}

	return result, true
}

// canonicalizeSlow is the original implementation: the whole document is rendered
// with the generator that writes benthos.yaml and parsed back, which leaves the
// file as the single definition of the observed shape. An earlier version
// reproduced yaml.v3's emitter decisions in Go instead and needed an ever-growing
// table of value shapes to refuse - canonicalizeFast above is that table, kept as
// a fast path rather than the only path since it still must decline shapes it
// cannot prove correct.
//
// On a render or parse error cfg is returned unchanged, so canonicalization can
// never make two equal configs look different.
func canonicalizeSlow(cfg BenthosServiceConfig) BenthosServiceConfig {
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
