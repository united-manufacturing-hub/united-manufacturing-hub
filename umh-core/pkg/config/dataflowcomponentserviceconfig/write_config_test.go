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
})
