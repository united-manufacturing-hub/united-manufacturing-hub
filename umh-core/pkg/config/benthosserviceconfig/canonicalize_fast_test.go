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
	"fmt"
	"math"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// yamlRoundTrip is the original canonicalization. fastNormalize must be
// indistinguishable from it whenever it accepts.
func yamlRoundTrip(v interface{}) (interface{}, error) {
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

// marshalConfig is a small helper so the benchmark file does not need its own
// yaml import just to report sizes.
func marshalConfig(cfg BenthosServiceConfig) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// bridgeConfigDeclining adds a value the fast path must refuse. Kept separate from
// bridgeConfig: one declining value makes the whole section fall back, which would
// silently turn every timing measurement into a measurement of the slow path.
func bridgeConfigDeclining(blocks int) BenthosServiceConfig {
	cfg := bridgeConfig(blocks)
	s7 := cfg.Input["s7comm"].(map[string]interface{})
	s7["preamble"] = "\nSELECT 1;\n"

	return cfg
}

// bridgeConfigOfBytes grows a bridge config until its YAML encoding reaches at
// least target bytes, so a test can ask for "a customer-sized bridge" directly.
func bridgeConfigOfBytes(target int) BenthosServiceConfig {
	blocks := 10
	for {
		cfg := bridgeConfig(blocks)

		encoded, err := yaml.Marshal(cfg)
		if err != nil || len(encoded) >= target || blocks > 100_000 {
			return cfg
		}

		blocks *= 2
	}
}

// slowCanonicalize is the original implementation, kept so the benchmark can time
// both paths without reaching into production code.
func slowCanonicalize(cfg BenthosServiceConfig) BenthosServiceConfig {
	slowMap := func(m map[string]interface{}) map[string]interface{} {
		if len(m) == 0 {
			return m
		}

		out, err := yamlRoundTrip(m)
		if err != nil {
			return m
		}

		result, ok := out.(map[string]interface{})
		if !ok {
			return m
		}

		return result
	}

	cfg.Input = slowMap(cfg.Input)
	cfg.Output = slowMap(cfg.Output)
	cfg.Pipeline = slowMap(cfg.Pipeline)
	cfg.Buffer = slowMap(cfg.Buffer)

	return cfg
}

// valuePositions wraps a value in each position where yaml makes a different
// emitter decision. A multi-line string starting with a space survives as a nested
// map value but loses its indentation inside a sequence element, so a table testing
// only one position cannot see it.
var valuePositions = []struct {
	name string
	wrap func(interface{}) interface{}
}{
	{"nested map value", func(v interface{}) interface{} {
		return map[string]interface{}{"v": v}
	}},
	{"sequence element", func(v interface{}) interface{} {
		return map[string]interface{}{"p": []interface{}{map[string]interface{}{"k": v}}}
	}},
}

var _ = Describe("canonicalize fast path", func() {
	// Asserts both halves of the contract: the fast path must be taken, and its
	// result must equal the round-trip. Without the acceptance check every spec here
	// would still pass if fastNormalize declined unconditionally.
	expectAccepted := func(label string, in interface{}) {
		slow, err := yamlRoundTrip(in)
		Expect(err).NotTo(HaveOccurred(), "%s: round-trip failed", label)

		fast, ok := fastNormalize(in)
		Expect(ok).To(BeTrue(), "%s: fast path declined; it must handle this shape", label)
		Expect(fast).To(Equal(slow), "%s: fast path disagrees with the YAML round-trip", label)
	}

	Describe("on a realistic bridge config", func() {
		It("produces the same result as the YAML round-trip for every section", func() {
			cfg := bridgeConfig(200)

			expectAccepted("Input", cfg.Input)
			expectAccepted("Pipeline", cfg.Pipeline)
			expectAccepted("Output", cfg.Output)
			expectAccepted("Buffer", cfg.Buffer)
			expectAccepted("CacheResources", cfg.CacheResources)
		})

		It("makes a Go-built config equal to its YAML-parsed twin", func() {
			desired := bridgeConfig(50)

			// Simulate what is read back from benthos.yaml.
			raw, err := yaml.Marshal(desired)
			Expect(err).NotTo(HaveOccurred())

			var observed BenthosServiceConfig
			Expect(yaml.Unmarshal(raw, &observed)).To(Succeed())

			Expect(NewComparator().ConfigsEqual(desired, observed)).To(BeTrue(),
				"a config and its YAML round-trip must compare equal")
		})

		It("stays correct when the fast path declines and falls back", func() {
			desired := bridgeConfigDeclining(20)

			// The fast path must refuse this one outright...
			_, ok := fastNormalize(desired.Input)
			Expect(ok).To(BeFalse(), "fixture no longer triggers a fallback")

			// ...and the fallback must still make it equal to its YAML twin.
			raw, err := yaml.Marshal(desired)
			Expect(err).NotTo(HaveOccurred())

			var observed BenthosServiceConfig
			Expect(yaml.Unmarshal(raw, &observed)).To(Succeed())

			Expect(NewComparator().ConfigsEqual(desired, observed)).To(BeTrue(),
				"the round-trip fallback must produce a correct comparison")
		})

		It("still reports a genuine difference", func() {
			a := bridgeConfig(10)
			b := bridgeConfig(10)
			b.Input["s7comm"].(map[string]interface{})["rack"] = 1

			Expect(NewComparator().ConfigsEqual(a, b)).To(BeFalse())
		})
	})

	Describe("type-representation drift", func() {
		// Each case names the value the round-trip produces: two paths agreeing says
		// nothing about what they agree on. The int/int64 split assumes a 64-bit build.
		DescribeTable("matches the YAML round-trip",
			func(value, expected interface{}) {
				for _, pos := range valuePositions {
					in := pos.wrap(value)
					want := pos.wrap(expected)

					slow, err := yamlRoundTrip(in)
					Expect(err).NotTo(HaveOccurred(), pos.name)
					Expect(slow).To(Equal(want),
						"%s: the round-trip does not produce the documented value", pos.name)

					fast, ok := fastNormalize(in)
					Expect(ok).To(BeTrue(), "%s: fast path declined; it must handle this shape", pos.name)
					Expect(fast).To(Equal(want), "%s: fast path disagrees with the round-trip", pos.name)
				}
			},
			Entry("[]string becomes []interface{}", []string{"a", "b"}, []interface{}{"a", "b"}),
			Entry("map[string]string becomes map[string]interface{}",
				map[string]string{"k": "v"}, map[string]interface{}{"k": "v"}),
			Entry("int64 collapses to int", int64(42), 42),
			Entry("int64 max", int64(math.MaxInt64), math.MaxInt),
			Entry("int64 min", int64(math.MinInt64), math.MinInt),
			Entry("uint collapses to int", uint(7), 7),
			Entry("uint at MaxUint32", uint(math.MaxUint32), int(math.MaxUint32)),
			Entry("uint64 at MaxInt64 still fits int", uint64(math.MaxInt64), math.MaxInt),
			Entry("uint64 past MaxInt64 stays uint64",
				uint64(math.MaxInt64)+1, uint64(math.MaxInt64)+1),
			Entry("uint64 max stays uint64", uint64(math.MaxUint64), uint64(math.MaxUint64)),
			Entry("integral float64 becomes int", float64(1000), 1000),
			Entry("integral float64 zero becomes int", float64(0), 0),
			Entry("negative zero float64 becomes int", math.Copysign(0, -1), 0),
			Entry("negative integral float64 becomes int", float64(-42), -42),
			// 2147483647.0 shortest-prints as "2.147483647e+09", which ParseInt
			// rejects, so an exponent in the text is what keeps a float a float.
			Entry("integral float64 large enough to print as an exponent stays float64",
				float64(math.MaxInt32), float64(math.MaxInt32)),
			Entry("float64 with a full mantissa keeps every digit",
				math.Nextafter(0.3, 1), math.Nextafter(0.3, 1)),
			Entry("float64 that stays a float in yaml", 1e21, 1e21),
			Entry("float64 infinity", math.Inf(1), math.Inf(1)),
			Entry("bool passes through", true, true),
			Entry("nil passes through", nil, nil),
			Entry("empty slice becomes empty []interface{}", []string{}, []interface{}{}),
			Entry("nil slice becomes empty []interface{}", []string(nil), []interface{}{}),
			Entry("empty map", map[string]string{}, map[string]interface{}{}),
			Entry("nested nil element", []interface{}{nil, "x"}, []interface{}{nil, "x"}),
			Entry("string that looks numeric stays a string", "0755", "0755"),
			Entry("string that looks boolean stays a string", "yes", "yes"),
			Entry("string with a trailing newline", "SELECT 1;\n", "SELECT 1;\n"),
			Entry("string with an interior blank line", "a\n\nb", "a\n\nb"),
			Entry("multiline string", "line1\nline2\n", "line1\nline2\n"),
			Entry("yaml-special characters", "a: b # c\n\ttab", "a: b # c\n\ttab"),
			Entry("unicode", "Grüße 🎉", "Grüße 🎉"),
			Entry("slice of maps",
				[]map[string]interface{}{{"a": []string{"b"}}},
				[]interface{}{map[string]interface{}{"a": []interface{}{"b"}}}),
			Entry("deeply nested",
				map[string]interface{}{
					"l1": map[string]interface{}{"l2": []interface{}{map[string]string{"l3": "v"}}},
				},
				map[string]interface{}{
					"l1": map[string]interface{}{"l2": []interface{}{map[string]interface{}{"l3": "v"}}},
				}),
		)

		// Checked in every position, so a shape that is only unsafe inside a sequence
		// element cannot pass by being tested somewhere harmless.
		DescribeTable("declines rather than guessing",
			func(value interface{}) {
				for _, pos := range valuePositions {
					_, ok := fastNormalize(pos.wrap(value))
					Expect(ok).To(BeFalse(),
						"%s: expected the fast path to decline %T(%q)", pos.name, value, value)
				}
			},
			Entry("float32", float32(1.1)),
			Entry("string with a leading newline", "\nSELECT 1;\n"),
			Entry("bare newline string", "\n"),
			Entry("multiline string with a leading space", " a\nb\n"),
			Entry("multiline string, later line indented deeper", "  a\n    b\n"),
			Entry("multiline string indented four", "    a\n    b\n"),
			Entry("struct", struct{ A int }{1}),
			Entry("pointer", new(int)),
		)

		It("declines non-string map keys rather than guessing", func() {
			_, ok := fastNormalize(map[interface{}]interface{}{0: "a"})
			Expect(ok).To(BeFalse())
		})
	})

})
