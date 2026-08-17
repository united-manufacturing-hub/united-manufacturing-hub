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
	"errors"
	"fmt"
	"math"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// bridgeConfig is shaped like a real TimescaleDB sql_raw write bridge: a large JS
// body, a large SQL block, an address list. blocks scales its size.
func bridgeConfig(blocks int) BenthosServiceConfig {
	var js, sql strings.Builder

	js.WriteString("// UNS -> TimescaleDB\nconst CONTRACT = \"pump\";\n")
	sql.WriteString("BEGIN;\nCREATE EXTENSION IF NOT EXISTS ltree;\n")

	for i := range blocks {
		fmt.Fprintf(&js, "if (msg.meta.tag_name === \"tag_%d\") { msg.meta.virtual_path = \"vp_%d\"; return msg; }\n", i, i)
		fmt.Fprintf(&sql, "INSERT INTO value_%d (topic_id, ts) VALUES ($1, $2) ON CONFLICT DO NOTHING;\n", i)
	}

	addresses := make([]string, 0, blocks)
	for i := range blocks {
		addresses = append(addresses, fmt.Sprintf("DB%d.I%d", i, i*2))
	}

	return BenthosServiceConfig{
		Input: map[string]interface{}{
			"uns": map[string]interface{}{
				"umh_topic":      "umh.v1.*",
				"broker_address": "localhost:9092",
				"consumer_group": "cg",
				"umh_topics":     []string{"umh.v1.a", "umh.v1.b"},
			},
			"s7comm": map[string]interface{}{
				"addresses": addresses,
				"rack":      0,
				"slot":      int64(2),
				"timeout":   10.5,
			},
		},
		Pipeline: map[string]interface{}{
			"processors": []map[string]interface{}{
				{"nodered_js": map[string]interface{}{"code": js.String()}},
				{"tag_processor": map[string]interface{}{"defaults": "return msg;", "enabled": true}},
			},
		},
		Output: map[string]interface{}{
			"sql_raw": map[string]interface{}{
				"driver":         "postgres",
				"dsn":            "postgres://u:p@localhost:5432/umh?sslmode=disable",
				"init_statement": sql.String(),
				"batching":       map[string]interface{}{"count": 1000, "period": "1s"},
				"max_in_flight":  8,
			},
		},
		Buffer:         map[string]interface{}{"none": map[string]interface{}{}},
		CacheResources: []map[string]interface{}{{"label": "c1", "memory": map[string]string{"ttl": "5m"}}},
		MetricsPort:    4195,
	}
}

// bridgeConfigMangled adds the strings yaml's block-scalar emitter does not write
// back verbatim: a leading newline is dropped, and inside a sequence element the
// common indentation is stripped from every line. Canonicalization has to reproduce
// that loss, because the mangled form is what ends up in benthos.yaml.
func bridgeConfigMangled(blocks int) BenthosServiceConfig {
	cfg := bridgeConfig(blocks)

	s7, ok := cfg.Input["s7comm"].(map[string]interface{})
	Expect(ok).To(BeTrue())

	s7["preamble"] = "\nSELECT 1;\n"

	procs, ok := cfg.Pipeline["processors"].([]map[string]interface{})
	Expect(ok).To(BeTrue())

	procs[0]["nodered_js"].(map[string]interface{})["indented"] = "  a\n    b\n"

	return cfg
}

// bridgeConfigOfBytes grows a bridge config until its YAML encoding reaches at
// least target bytes, so a test can ask for "a customer-sized bridge" directly.
func bridgeConfigOfBytes(target int) BenthosServiceConfig {
	blocks := 10
	for {
		cfg := bridgeConfig(blocks)

		encoded, err := marshalConfig(cfg)
		if err != nil || len(encoded) >= target || blocks > 100_000 {
			return cfg
		}

		blocks *= 2
	}
}

// failingMarshaler makes yaml.Marshal return an error rather than panic, which is
// the only way to reach canonicalize's render-error path on purpose.
type failingMarshaler struct{}

func (failingMarshaler) MarshalYAML() (interface{}, error) {
	return nil, errors.New("no")
}

// marshalConfig is a small helper so the benchmark file does not need its own yaml
// import just to report sizes.
func marshalConfig(cfg BenthosServiceConfig) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// sectionRoundTrip is the canonicalization this package used before: each section
// serialized on its own rather than in its place in the document. Kept so the
// benchmark can time both and so the block-scalar specs can show the difference.
func sectionRoundTrip(cfg BenthosServiceConfig) BenthosServiceConfig {
	section := func(m map[string]interface{}) map[string]interface{} {
		if len(m) == 0 {
			return m
		}

		encoded, err := yaml.Marshal(m)
		if err != nil {
			return m
		}

		var decoded map[string]interface{}
		if err := yaml.Unmarshal(encoded, &decoded); err != nil {
			return m
		}

		return decoded
	}

	resources := func(s []map[string]interface{}) []map[string]interface{} {
		if len(s) == 0 {
			return s
		}

		encoded, err := yaml.Marshal(s)
		if err != nil {
			return s
		}

		var decoded []map[string]interface{}
		if err := yaml.Unmarshal(encoded, &decoded); err != nil {
			return s
		}

		return decoded
	}

	cfg.Input = section(cfg.Input)
	cfg.Output = section(cfg.Output)
	cfg.Pipeline = section(cfg.Pipeline)
	cfg.Buffer = section(cfg.Buffer)
	cfg.CacheResources = resources(cfg.CacheResources)
	cfg.RateLimitResources = resources(cfg.RateLimitResources)

	return cfg
}

// templatedFrom returns the config the way a bridge's config reaches the comparator:
// RenderTemplate serializes the config struct, runs the text through text/template
// and parses it back, so the free-form maps hold the types a parse produces rather
// than the types Go code would build. The shape of that document differs from the
// one the generator writes, which is the point - both have to end up equal to the
// file without canonicalizing anything.
func templatedFrom(cfg BenthosServiceConfig) (BenthosServiceConfig, error) {
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return BenthosServiceConfig{}, err
	}

	var out BenthosServiceConfig
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return BenthosServiceConfig{}, err
	}

	return out, nil
}

// observedFrom is renderAndParse with the errors asserted away, for use in specs.
func observedFrom(cfg BenthosServiceConfig) BenthosServiceConfig {
	observed, err := renderAndParse(cfg)
	Expect(err).NotTo(HaveOccurred())

	return observed
}

// renderAndParse returns the config as the agent reads it back after writing cfg to
// benthos.yaml: rendered by the generator that writes the file, then extracted the
// way BenthosService.GetConfig extracts it. It is the independent oracle for
// canonicalize - deriving it from canonicalize itself would assert nothing.
func renderAndParse(cfg BenthosServiceConfig) (BenthosServiceConfig, error) {
	text, err := NewGenerator().RenderConfig(cfg)
	if err != nil {
		return BenthosServiceConfig{}, err
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return BenthosServiceConfig{}, err
	}

	observed := BenthosServiceConfig{}

	if input, ok := doc["input"].(map[string]interface{}); ok {
		observed.Input = input
	}

	if pipeline, ok := doc["pipeline"].(map[string]interface{}); ok {
		observed.Pipeline = pipeline
	}

	if output, ok := doc["output"].(map[string]interface{}); ok {
		observed.Output = output
	}

	if buffer, ok := doc["buffer"].(map[string]interface{}); ok {
		observed.Buffer = buffer
	}

	observed.CacheResources = append(observed.CacheResources, asList(doc["cache_resources"])...)
	observed.RateLimitResources = append(observed.RateLimitResources, asList(doc["rate_limit_resources"])...)

	if logger, ok := doc["logger"].(map[string]interface{}); ok {
		if level, ok := logger["level"].(string); ok {
			observed.DebugLevel = (level == "DEBUG")
		}
	}

	return observed, nil
}

func asList(v interface{}) []map[string]interface{} {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(list))

	for _, item := range list {
		if entry, ok := item.(map[string]interface{}); ok {
			result = append(result, entry)
		}
	}

	return result
}

// valuePositions puts a value in each position where yaml makes a different emitter
// decision. A multi-line string starting with a space survives as a nested map value
// but loses its indentation inside a sequence element, so a table testing only one
// position cannot see it.
var valuePositions = []struct {
	name  string
	build func(interface{}) BenthosServiceConfig
}{
	{"nested map value", func(v interface{}) BenthosServiceConfig {
		return BenthosServiceConfig{
			Input:  map[string]interface{}{"uns": map[string]interface{}{"v": v}},
			Output: map[string]interface{}{"stdout": map[string]interface{}{}},
		}
	}},
	{"sequence element", func(v interface{}) BenthosServiceConfig {
		return BenthosServiceConfig{
			Input: map[string]interface{}{"uns": map[string]interface{}{}},
			Pipeline: map[string]interface{}{
				"processors": []interface{}{map[string]interface{}{"k": v}},
			},
			Output: map[string]interface{}{"stdout": map[string]interface{}{}},
		}
	}},
}

var _ = Describe("canonicalize", func() {
	// The property that matters: a config built in Go must compare equal to the
	// config the agent reads back after writing it. Anything else re-applies the
	// config on every tick.
	expectSurvivesTheFile := func(desired BenthosServiceConfig, label string) {
		Expect(NewComparator().ConfigsEqual(desired, observedFrom(desired))).To(BeTrue(),
			"%s: a config must equal what is read back from the file it renders to", label)

		// Also with the arguments the other way round: two call sites in
		// pkg/communicator/actions pass the observed config first.
		Expect(NewComparator().ConfigsEqual(observedFrom(desired), desired)).To(BeTrue(),
			"%s: argument order must not decide the outcome", label)
	}

	Describe("on a realistic bridge config", func() {
		It("makes a Go-built config equal to the one read back from its file", func() {
			expectSurvivesTheFile(bridgeConfig(50), "bridge")
		})

		It("still reports a genuine difference", func() {
			a := bridgeConfig(10)
			b := bridgeConfig(10)
			b.Input["s7comm"].(map[string]interface{})["rack"] = 1

			Expect(NewComparator().ConfigsEqual(a, b)).To(BeFalse())
		})

		// getProcessors only recognizes a []interface{}, and reports "no processors" for
		// any other slice type. Two configs that both hold their processors as
		// []map[string]interface{} therefore both look processor-less, and comparing them
		// before canonicalization would skip the processors entirely.
		It("reports a difference in processors that are not held as []interface{}", func() {
			a := bridgeConfig(10)
			b := bridgeConfig(10)

			procs, ok := b.Pipeline["processors"].([]map[string]interface{})
			Expect(ok).To(BeTrue(), "fixture no longer holds its processors as []map[string]interface{}")

			procs[1]["tag_processor"].(map[string]interface{})["defaults"] = "return null;"

			Expect(NewComparator().ConfigsEqual(a, b)).To(BeFalse())
		})

		It("leaves sections the config does not have alone", func() {
			cfg := bridgeConfig(5)
			cfg.CacheResources = nil
			cfg.RateLimitResources = nil

			canonical := canonicalize(cfg)

			Expect(canonical.CacheResources).To(BeNil())
			Expect(canonical.RateLimitResources).To(BeNil())
		})
	})

	// The write does not give these strings back unchanged: a leading newline is
	// dropped, and in a sequence element the common indentation is stripped from every
	// line. Canonicalization has to reproduce the loss rather than refuse the value,
	// which is what the previous implementation's decline table did.
	Describe("strings the file does not preserve", func() {
		It("reproduces what the written file does to them", func() {
			expectSurvivesTheFile(bridgeConfigMangled(20), "mangled")
		})

		It("is what makes them compare equal, not luck", func() {
			desired := bridgeConfigMangled(5)
			observed := observedFrom(desired)

			// Without canonicalization the desired config differs from the file, which
			// is the difference that used to be re-applied forever.
			raw := NewNormalizer().NormalizeConfig(desired)
			Expect(configsEqualNormalized(raw, NewNormalizer().NormalizeConfig(observed))).
				To(BeFalse(), "fixture no longer contains a string the file changes")
		})

		// Rendering the document is a simplification over round-tripping each section,
		// not a change of outcome: yaml.v3's block scalars carry an explicit indentation
		// indicator, so what a value parses back as does not depend on how deeply the
		// document nests it. Pinned here so a future change that does alter an outcome
		// has to say so.
		It("agrees with round-tripping each section on its own", func() {
			desired := bridgeConfigMangled(5)

			perSection := sectionRoundTrip(NewNormalizer().NormalizeConfig(desired))
			Expect(configsEqualNormalized(perSection, canonicalize(NewNormalizer().NormalizeConfig(desired)))).
				To(BeTrue())
		})

		It("still spots a real difference in a config that contains them", func() {
			a := bridgeConfigMangled(10)
			b := bridgeConfigMangled(10)
			b.Input["s7comm"].(map[string]interface{})["rack"] = 1

			Expect(NewComparator().ConfigsEqual(a, b)).To(BeFalse())
		})
	})

	Describe("type-representation drift", func() {
		// Each entry is a Go value that yaml reads back as something else. The struct
		// and pointer entries are shapes the previous fast path could not reproduce and
		// had to hand to the slow path.
		DescribeTable("survives the write and read back",
			func(value interface{}) {
				for _, pos := range valuePositions {
					expectSurvivesTheFile(pos.build(value), pos.name)
				}
			},
			Entry("[]string", []string{"a", "b"}),
			Entry("map[string]string", map[string]string{"k": "v"}),
			Entry("int64", int64(42)),
			Entry("int64 max", int64(math.MaxInt64)),
			Entry("int64 min", int64(math.MinInt64)),
			Entry("uint", uint(7)),
			Entry("uint64 past MaxInt64", uint64(math.MaxInt64)+1),
			Entry("uint64 max", uint64(math.MaxUint64)),
			Entry("uint16", uint16(502)),
			Entry("float32", float32(1.1)),
			Entry("integral float64", float64(1000)),
			Entry("negative zero float64", math.Copysign(0, -1)),
			Entry("float64 with a full mantissa", math.Nextafter(0.3, 1)),
			Entry("float64 infinity", math.Inf(1)),
			Entry("bool", true),
			Entry("nil", nil),
			Entry("empty slice", []string{}),
			Entry("nil slice", []string(nil)),
			Entry("empty map", map[string]string{}),
			Entry("struct", struct{ A int }{1}),
			Entry("pointer", new(int)),
			Entry("string that looks numeric", "0755"),
			Entry("string that looks boolean", "yes"),
			Entry("string with a trailing newline", "SELECT 1;\n"),
			Entry("string with an interior blank line", "a\n\nb"),
			Entry("bare newline string", "\n"),
			Entry("multiline string, later line indented deeper", "  a\n    b\n"),
			Entry("yaml-special characters", "a: b # c\n\ttab"),
			Entry("unicode", "Grüße 🎉"),
			Entry("slice of maps", []map[string]interface{}{{"a": []string{"b"}}}),
		)
	})

	// Inside a sequence element yaml.v3 writes a string that begins with a space or a
	// newline as a block scalar with an indentation indicator it cannot read back, so
	// the rendered document does not parse. Canonicalization cannot say what such a
	// config looks like on disk, and has to leave it alone rather than guess.
	//
	// The rendered file is what the agent writes to benthos.yaml, so benthos cannot
	// read such a config either. That is a defect in the generator, not here.
	Describe("when the rendered document does not parse", func() {
		unparseable := func(s string) BenthosServiceConfig {
			return BenthosServiceConfig{
				Input: map[string]interface{}{"uns": map[string]interface{}{}},
				Pipeline: map[string]interface{}{
					"processors": []interface{}{map[string]interface{}{"code": s}},
				},
				Output: map[string]interface{}{"stdout": map[string]interface{}{}},
			}
		}

		DescribeTable("returns the config unchanged and still compares it as equal to itself",
			func(s string) {
				cfg := unparseable(s)

				text, err := NewGenerator().RenderConfig(cfg)
				Expect(err).NotTo(HaveOccurred())

				var doc map[string]interface{}
				Expect(yaml.Unmarshal([]byte(text), &doc)).NotTo(Succeed(),
					"the generator now writes a parseable document; move this case back to the table above")

				Expect(canonicalize(cfg)).To(Equal(cfg))
				Expect(NewComparator().ConfigsEqual(cfg, cfg)).To(BeTrue())
			},
			Entry("string with a leading newline", "\nSELECT 1;\n"),
			Entry("multiline string with a leading space", " a\nb\n"),
		)
	})

	Describe("when the config cannot be rendered", func() {
		// Anything that stops the render has to leave the config untouched: a config
		// that is equal to itself must never be reported as different.
		unrenderable := func() BenthosServiceConfig {
			return BenthosServiceConfig{
				Input:  map[string]interface{}{"uns": map[string]interface{}{"v": failingMarshaler{}}},
				Output: map[string]interface{}{"stdout": map[string]interface{}{}},
			}
		}

		It("returns the config unchanged", func() {
			cfg := unrenderable()

			_, err := NewGenerator().RenderConfig(cfg)
			Expect(err).To(HaveOccurred(), "fixture is renderable after all")

			Expect(canonicalize(cfg)).To(Equal(cfg))
		})

		It("still compares two equal configs as equal", func() {
			Expect(NewComparator().ConfigsEqual(unrenderable(), unrenderable())).To(BeTrue())
		})
	})

	It("is deterministic", func() {
		cfg := NewNormalizer().NormalizeConfig(bridgeConfig(20))

		Expect(canonicalize(cfg)).To(Equal(canonicalize(cfg)))
	})

	// ConfigsEqual canonicalizes nothing when the two sides already agree, and this is
	// what makes that worth doing: a bridge's config reaches the comparator parsed from
	// rendered template text, so it is already in the shape the file holds. Losing this
	// puts a YAML serialization of every component's config back into every tick.
	It("is not needed at all for a config that came from a template", func() {
		templated, err := templatedFrom(bridgeConfig(50))
		Expect(err).NotTo(HaveOccurred())

		norm := NewNormalizer()

		Expect(configsEqualNormalized(
			norm.NormalizeConfig(templated),
			norm.NormalizeConfig(observedFrom(templated)),
		)).To(BeTrue(), "a templated config no longer matches its own file without canonicalization")
	})
})
