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

package dataflowcomponentserviceconfig_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

var _ = Describe("write_config", func() {

	Describe("splitTopics (via ToWriteConfig)", func() {
		DescribeTable("splits topic strings correctly",
			func(input string, expected []string) {
				cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: input},
				}
				result := cfg.ToWriteConfig().Topics
				Expect(result).To(Equal(expected))
			},
			Entry("newline-separated", "a\nb\nc", []string{"a", "b", "c"}),
			Entry("mixed newline and comma", "a,b\nc", []string{"a,b", "c"}),
			Entry("trims whitespace", "  a  \n  b  ", []string{"a", "b"}),
			Entry("skips empty lines", "a\n\nb", []string{"a", "b"}),
			Entry("single topic", "umh.v1.factory.*", []string{"umh.v1.factory.*"}),
			Entry("empty string", "", []string{}),
		)
	})

	Describe("HasOutput", func() {
		It("returns false when Destination is empty", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{}
			Expect(cfg.HasOutput()).To(BeFalse())
		})

		It("returns false when Destination has no Protocol or Code", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{},
			}
			Expect(cfg.HasOutput()).To(BeFalse())
		})

		It("returns true when Destination has Protocol", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
					Protocol: "http_client",
					Code:     "url: http://example.com\nverb: POST",
				},
			}
			Expect(cfg.HasOutput()).To(BeTrue())
		})
	})

	Describe("ToDataflowComponentServiceConfig", func() {
		It("builds UNS input with provided topics", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
					Protocol: "stdout",
				},
			}
			svc := cfg.ToDataflowComponentServiceConfig("my-bridge")
			Expect(svc.BenthosConfig.Input).To(HaveKey("uns"))
			uns := svc.BenthosConfig.Input["uns"].(map[string]any)
			Expect(uns["consumer_group"]).To(Equal("my-bridge"))
			Expect(uns["umh_topics"]).To(ContainElement("umh.v1.factory.*"))
		})

		It("uses sentinel when Source.Topics is empty and Destination is set", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: ""},
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
					Protocol: "stdout",
				},
			}
			svc := cfg.ToDataflowComponentServiceConfig("my-bridge")
			uns := svc.BenthosConfig.Input["uns"].(map[string]any)
			Expect(uns["umh_topics"]).To(ContainElement(dataflowcomponentserviceconfig.PlaceholderUMHTopicUnset))
		})

		It("produces an empty config when no output is configured", func() {
			// A write flow with no destination never starts and renders an empty
			// benthos.yaml. The rendered config must be empty too — including the
			// pipeline — so it matches the observed empty config and does not report
			// permanent "Processors differ" divergence every tick.
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source:     dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
				Processing: dataflowcomponentserviceconfig.WriteConfigProcessing{Code: "return msg;"},
			}
			svc := cfg.ToDataflowComponentServiceConfig("my-bridge")
			Expect(svc.BenthosConfig.Input).To(BeNil())
			Expect(svc.BenthosConfig.Output).To(BeNil())
			Expect(svc.BenthosConfig.Pipeline).To(BeEmpty())
			Expect(svc).To(Equal(dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}))
		})

		It("defaults Processing.Code to 'return msg;' when empty", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
					Protocol: "stdout",
				},
			}
			svc := cfg.ToDataflowComponentServiceConfig("b")
			processors := svc.BenthosConfig.Pipeline["processors"].([]any)
			Expect(processors).To(HaveLen(1))
			proc := processors[0].(map[string]any)
			Expect(proc["nodered_js"].(map[string]any)["code"]).To(Equal("return msg;"))
		})
	})

	Describe("ToWriteConfig round-trip", func() {
		It("preserves all fields through ToWriteConfig", func() {
			input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{
					Topics: "umh.v1.*\numh.v1.other.*",
				},
				Processing: dataflowcomponentserviceconfig.WriteConfigProcessing{
					Type: "nodered_js",
					Code: "return msg;",
				},
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
					Protocol: "kafka",
					Code:     "addresses:\n  - localhost:9092",
				},
				Extra: &dataflowcomponentserviceconfig.WriteConfigExtra{
					Code: "buffer:\n  memory: {}\n",
				},
			}
			got := input.ToWriteConfig()
			Expect(got.Topics).To(ConsistOf("umh.v1.*", "umh.v1.other.*"))
			Expect(got.Processing.Code).To(Equal("return msg;"))
			Expect(got.Destination.Protocol).To(Equal("kafka"))
			Expect(got.Extra).NotTo(BeNil())
			Expect(got.Extra.Code).To(ContainSubstring("buffer"))
		})

		It("injects buffer from Extra into BenthosConfig", func() {
			input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
				Extra: &dataflowcomponentserviceconfig.WriteConfigExtra{
					Code: "buffer:\n  memory: {}\n",
				},
			}
			svc := input.ToDataflowComponentServiceConfig("b")
			Expect(svc.BenthosConfig.Buffer).To(HaveKey("memory"))
		})
	})

	Describe("topic location", func() {
		DescribeTable("reports whether a topic can only match beneath the bridge location",
			func(topic, locationPath string, expected bool) {
				input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: topic},
				}
				accepted := input.ToWriteConfig().ValidateTopics(locationPath, nil) == nil

				Expect(accepted).To(Equal(expected))
			},
			Entry("direct child", `^umh\.v1\.acme\.cologne\._raw\.`, "acme.cologne", true),
			Entry("deep child", `^umh\.v1\.acme\.cologne\.a\.b\._raw\.c`, "acme.cologne", true),
			Entry("child of a bridge one level up", `^umh\.v1\.acme\.cologne\._raw\.`, "acme", true),
			Entry("sibling of a bridge one level up", `^umh\.v1\.acme\.barcelona\._raw\.`, "acme", true),
			Entry("sibling branch", `^umh\.v1\.acme\.barcelona\._raw\.`, "acme.cologne", false),
			Entry("above the bridge location", `^umh\.v1\.acme\._raw\.`, "acme.cologne", false),
			Entry("shared prefix but different level", `^umh\.v1\.acme\.cologne2\._raw\.`, "acme.cologne", false),
			Entry("bridge location itself", `^umh\.v1\.acme\.cologne$`, "acme.cologne", false),
			Entry("alternation reaching a sibling", `^umh\.v1\.acme\.(?:cologne|barcelona)\._raw\.`, "acme.cologne", false),
			Entry("unescaped location dot also matches a sibling such as acme-cologne", `^umh.v1.acme.cologne\._raw\.`, "acme.cologne", false),
			Entry("unescaped location dots", `^umh.v1.acme.cologne.line-4._raw\.`, "acme.cologne.line-4", false),
			Entry("unescaped dot ending a level beneath the location also matches a level such as x-_raw", `^umh\.v1\.acme\.x._raw\.`, "acme", false),
			Entry("location levels become levels beneath a bridge moved up", `^umh\.v1\.acme\.cologne\.foo\.bar\.baz\._raw\.motor_running`, "acme.cologne.foo.bar", true),
			Entry("unescaped dot in place of a location character", `^umh\.v1\.acm.\.cologne\._raw\.`, "acme.cologne", false),
			Entry("wildcard in place of a location level", `^umh\.v1\.acme\..*\._raw\.`, "acme.cologne", false),
			Entry("wildcard across location levels", `^umh\.v1\.acme.*\._raw\.`, "acme.cologne", false),
			Entry("unanchored pattern also matches mid-topic", `umh\.v1\.acme\.cologne\._raw\.`, "acme.cologne", false),
			Entry("location with regex metacharacters taken literally", `^umh\.v1\.acme\.line-4\._raw\.`, "acme.line-4", true),
			Entry("case-insensitive pattern also matches other locations", `(?i)^umh\.v1\.acme\.cologne\._raw\.`, "acme.cologne", false),
			Entry("empty location accepts any level naming a contract", `^umh\.v1\.anything\._raw\.`, "", true),
		)
	})

	Describe("topic data contract", func() {
		declared := []string{"_pump_v1", "_pump_v2", "_pump_v3", "_pump_v4", "_pump_v5", "_valve_v1"}

		DescribeTable("reports whether a topic names exactly one existing data contract",
			func(topic string, expected bool) {
				input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: topic},
				}
				accepted := input.ToWriteConfig().ValidateTopics("acme.cologne.line-4", declared) == nil

				Expect(accepted).To(Equal(expected))
			},
			Entry("one machine", `^umh\.v1\.acme\.cologne\.line-4\.pump-01\._pump_v1\.`, true),
			Entry("every level beneath the bridge", `^umh\.v1\.acme\.cologne\.line-4\.(?:[^._][^.]*\.)*_pump_v1\.`, true),
			Entry("one tag on every level beneath the bridge", `^umh\.v1\.acme\.cologne\.line-4\.(?:[^._][^.]*\.)*_pump_v1\.motor\.current`, true),
			Entry("built-in contract", `^umh\.v1\.acme\.cologne\.line-4\._raw\.`, true),
			Entry("a pattern that matches no topic, such as an error message pasted back in", `^umh\.v1\.acme\.cologne\.line-4\.: ^umh\.v1\..*\._pump(?:_v\d+)?\.`, false),
			Entry("a pattern that requires the topic to end before more text", `^umh\.v1\.acme\.cologne\.line-4\._raw\.$x`, false),
			Entry("a branch that matches no topic next to one that does", `^umh\.v1\.acme\.cologne\.line-4\.(?:_raw\.$x|_pump_v1\.)`, true),
			Entry("built-in historian contract", `^umh\.v1\.acme\.cologne\.line-4\._historian\.temperature`, true),
			Entry("specific virtual path and tag", `^umh\.v1\.acme\.cologne\.line-4\._pump_v1\.motor\.current`, true),
			Entry("virtual path subtree", `^umh\.v1\.acme\.cologne\.line-4\._pump_v1\.motor\..*`, true),
			Entry("every tag of the contract", `^umh\.v1\.acme\.cologne\.line-4\._pump_v1\..*`, true),
			Entry("explicit version range", `^umh\.v1\.acme\.cologne\.line-4\._pump_v[1-5]\.`, true),
			Entry("explicit version alternation", `^umh\.v1\.acme\.cologne\.line-4\._pump_v(?:1|3)\.`, true),
			Entry("custom contract", `^umh\.v1\.acme\.cologne\.line-4\._valve_v1\.`, true),
			Entry("contract not declared", `^umh\.v1\.acme\.cologne\.line-4\._pump_v6\.`, false),
			Entry("version range reaching undeclared versions", `^umh\.v1\.acme\.cologne\.line-4\._pump_v[1-8]\.`, false),
			Entry("wildcard over versions", `^umh\.v1\.acme\.cologne\.line-4\._pump_v\d+\.`, false),
			Entry("any version", `^umh\.v1\.acme\.cologne\.line-4\._pump_v.*\.`, false),
			Entry("optional version", `^umh\.v1\.acme\.cologne\.line-4\._pump(?:_v\d+)?\.`, false),
			Entry("contract without a version", `^umh\.v1\.acme\.cologne\.line-4\._pump\.`, false),
			Entry("unescaped dot after the contract also matches _pump_v10", `^umh\.v1\.acme\.cologne\.line-4\._pump_v1.*`, false),
			Entry("unescaped topic", `umh.v1.acme.cologne.line-4._pump_v1.*`, false),
			Entry("contract not followed by a dot", `^umh\.v1\.acme\.cologne\.line-4\._pump_v1`, false),
			Entry("two different contracts", `^umh\.v1\.acme\.cologne\.line-4\._(?:pump|valve)_v1\.`, false),
			Entry("no contract", `^umh\.v1\.acme\.cologne\.line-4\.pump-01\.temperature`, false),
			Entry("wildcard instead of a contract", `^umh\.v1\.acme\.cologne\.line-4\..*`, false),
			Entry("wildcard contract", `^umh\.v1\.acme\.cologne\.line-4\._.*\.`, false),
			Entry("any levels as .* also match other contracts, such as _raw.x._pump_v1.motor.current", `^umh\.v1\.acme\.cologne\.line-4\..*\._pump_v1\.motor\.current`, false),
			Entry("wildcard levels reaching into the contract", `^umh\.v1\.acme\.cologne\.line-4\.a.*_pump_v1\.`, false),
			Entry("contract wildcard after any levels", `^umh\.v1\.acme\.cologne\.line-4\.(?:[^.]*\.)*_pump_v1\.`, false),
			Entry("underscore-prefixed level read as the contract", `^umh\.v1\.acme\.cologne\.line-4\._area\._raw\.`, false),
			Entry("empty level", `^umh\.v1\.acme\.cologne\.line-4\.\._raw\.`, false),
			Entry("unescaped dot ending a level beneath the location also matches a level such as x-_pump_v1", `^umh\.v1\.acme\.cologne\.line-4\.x._pump_v1\.`, false),
		)
	})

	Describe("underscore-prefixed bridge location", func() {
		validate := func(topic string) error {
			input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: topic},
			}

			return input.ToWriteConfig().ValidateTopics("acme._site", nil)
		}

		It("reads the contract beneath the location, not the location level", func() {
			Expect(validate(`^umh\.v1\.acme\._site\._raw\.`)).To(Succeed())
		})

		It("rejects a topic naming no contract beneath the location", func() {
			Expect(validate(`^umh\.v1\.acme\._site\.temperature`)).NotTo(Succeed())
		})
	})

	Describe("ValidateTopics", func() {
		validate := func(topics, locationPath string) error {
			input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: topics},
			}

			return input.ToWriteConfig().ValidateTopics(locationPath, nil)
		}

		It("reports every offending topic", func() {
			err := validate(`^umh\.v1\.acme\.cologne\._raw\.foo`+"\n"+`^umh\.v1\.acme\.cologne\._raw\.bar`, "acme.cologne.sublevel")
			Expect(err).To(MatchError(ContainSubstring(`^umh\.v1\.acme\.cologne\._raw\.foo`)))
			Expect(err).To(MatchError(ContainSubstring(`^umh\.v1\.acme\.cologne\._raw\.bar`)))
			Expect(err).To(MatchError(ContainSubstring(`^umh\.v1\.acme\.cologne\.sublevel\.`)))
		})

		It("describes each offending topic in its own sentence and ends with a valid example", func() {
			err := validate(`^umh\.v1\.elsewhere\._raw\.foo`+"\n"+`^umh\.v1\.acme\.cologne\.bar`+"\n"+
				`umh\.v1\.acme\.cologne\._raw\.`+"\n"+`^umh\.v1\.acme\.cologne\.(`+"\n"+
				`^umh\.v1\.acme\.cologne\._raw\.$x`, "acme.cologne")
			Expect(err).To(MatchError(`the write flow topics need changes: ` +
				`topic "^umh\.v1\.elsewhere\._raw\.foo" can match topics outside this bridge's location acme.cologne; start it with ^umh\.v1\.acme\.cologne\.; ` +
				`topic "^umh\.v1\.acme\.cologne\.bar" must name exactly one data contract after the location, followed by \.; available: _raw, _historian; list versions explicitly, like _pump_v[1-2]\.; ` +
				`topic "umh\.v1\.acme\.cologne\._raw\." must start with ^, otherwise it also matches topics that only contain it; ` +
				`topic "^umh\.v1\.acme\.cologne\.(" is not a valid regular expression: missing closing ); ` +
				`topic "^umh\.v1\.acme\.cologne\._raw\.$x" cannot match any topic, so the write flow would receive no messages. ` +
				`Example of a valid topic: ^umh\.v1\.acme\.cologne\._historian\.`))
		})

		It("names the contracts a topic may use", func() {
			err := validate(`^umh\.v1\.acme\.cologne\._nope\.foo`, "acme.cologne")
			Expect(err).To(MatchError(ContainSubstring("_raw")))
			Expect(err).To(MatchError(ContainSubstring("_historian")))
		})

		It("explains that a declared data contract without a version needs one", func() {
			input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: `^umh\.v1\.acme\.cologne\._pump\.`},
			}

			err := input.ToWriteConfig().ValidateTopics("acme.cologne", []string{"_pump", "_valve_v1"})
			Expect(err).To(MatchError(ContainSubstring(`available: _raw, _historian, _valve_v1;`)))
			Expect(err).To(MatchError(ContainSubstring(
				`data contract "_pump" has no version; rename it with one, like _pump_v1, before a write flow can use it`)))
			Expect(err).To(MatchError(HaveSuffix(`Example of a valid topic: ^umh\.v1\.acme\.cologne\._valve_v1\.`)))
		})

		It("tells a bridge without a location to start beneath the UNS prefix", func() {
			err := validate(`^other\._raw\.`, "")
			Expect(err).To(MatchError(ContainSubstring(`can match topics outside the UNS; start it with ^umh\.v1\.`)))
		})

		It("accepts a versioned data contract next to an unversioned declared one", func() {
			input := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: `^umh\.v1\.acme\.cologne\._valve_v1\.`},
			}

			Expect(input.ToWriteConfig().ValidateTopics("acme.cologne", []string{"_pump", "_valve_v1"})).To(Succeed())
		})

		It("accepts the placeholder used when no topic was configured", func() {
			Expect(validate(dataflowcomponentserviceconfig.PlaceholderUMHTopicUnset, "acme.cologne")).To(Succeed())
		})
	})
})
