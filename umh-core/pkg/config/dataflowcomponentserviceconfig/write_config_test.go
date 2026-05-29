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
					InputTopics: input,
				}
				result := cfg.ToWriteConfig().InputTopics
				Expect(result).To(Equal(expected))
			},
			Entry("newline-separated", "a\nb\nc", []string{"a", "b", "c"}),
			Entry("comma-separated", "a,b,c", []string{"a", "b", "c"}),
			Entry("mixed newline and comma", "a,b\nc", []string{"a", "b", "c"}),
			Entry("trims whitespace", "  a  \n  b  ", []string{"a", "b"}),
			Entry("skips empty lines", "a\n\nb", []string{"a", "b"}),
			Entry("all commas (only-commas → empty)", ",,,,", []string{}),
			Entry("single topic", "umh.v1.factory.*", []string{"umh.v1.factory.*"}),
			Entry("empty string", "", []string{}),
		)
	})

	Describe("HasOutput", func() {
		It("returns false when Output is nil", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{}
			Expect(cfg.HasOutput()).To(BeFalse())
		})

		It("returns false when Output is empty map", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Output: map[string]any{},
			}
			Expect(cfg.HasOutput()).To(BeFalse())
		})

		It("returns true when Output has entries", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Output: map[string]any{"http_client": map[string]any{"url": "http://example.com"}},
			}
			Expect(cfg.HasOutput()).To(BeTrue())
		})
	})

	Describe("ToDataflowComponentServiceConfig", func() {
		It("builds UNS input with provided topics", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				InputTopics: "umh.v1.factory.*",
				Output:      map[string]any{"stdout": map[string]any{}},
			}
			svc := cfg.ToDataflowComponentServiceConfig("my-bridge")
			Expect(svc.BenthosConfig.Input).To(HaveKey("uns"))
			uns := svc.BenthosConfig.Input["uns"].(map[string]any)
			Expect(uns["consumer_group"]).To(Equal("my-bridge"))
			Expect(uns["umh_topics"]).To(ContainElement("umh.v1.factory.*"))
		})

		It("uses sentinel when InputTopics is empty and Output is set", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				InputTopics: "",
				Output:      map[string]any{"stdout": map[string]any{}},
			}
			svc := cfg.ToDataflowComponentServiceConfig("my-bridge")
			uns := svc.BenthosConfig.Input["uns"].(map[string]any)
			Expect(uns["umh_topics"]).To(ContainElement(dataflowcomponentserviceconfig.PlaceholderUMHTopicUnset))
		})

		It("produces no input when no output is configured", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				InputTopics: "umh.v1.factory.*",
			}
			svc := cfg.ToDataflowComponentServiceConfig("my-bridge")
			Expect(svc.BenthosConfig.Input).To(BeNil())
		})

		It("defaults ProcessingNoderedJS to 'return msg;' when empty", func() {
			cfg := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				InputTopics: "umh.v1.*",
				Output:      map[string]any{"stdout": map[string]any{}},
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
				InputTopics:         "umh.v1.*\numh.v1.other.*",
				ProcessingNoderedJS: "return msg;",
				Output:              map[string]any{"kafka": map[string]any{}},
				Buffer:              map[string]any{"none": map[string]any{}},
			}
			got := input.ToWriteConfig()
			Expect(got.InputTopics).To(ConsistOf("umh.v1.*", "umh.v1.other.*"))
			Expect(got.ProcessingNoderedJS).To(Equal("return msg;"))
			Expect(got.Output).To(HaveKey("kafka"))
			Expect(got.Buffer).To(HaveKey("none"))
		})
	})
})
