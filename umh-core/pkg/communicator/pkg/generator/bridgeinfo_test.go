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

package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("extractDataContractsFromPipeline", func() {
	It("returns nil when the pipeline has no processors", func() {
		Expect(extractDataContractsFromPipeline(map[string]any{})).To(BeNil())
	})

	It("returns nil when processors is not a slice", func() {
		pipeline := map[string]any{"processors": "not-a-slice"}
		Expect(extractDataContractsFromPipeline(pipeline)).To(BeNil())
	})

	It("extracts a single contract from tag_processor defaults", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"tag_processor": map[string]any{
						"defaults": `msg.meta.location_path = "{{ .location_path }}";
msg.meta.data_contract = "_count-model_v1";
msg.meta.tag_name = "my_tag";
return msg;`,
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(Equal([]string{"_count-model_v1"}))
	})

	It("collects contracts from multiple conditions in first-seen order", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"tag_processor": map[string]any{
						"defaults": "",
						"conditions": []any{
							map[string]any{
								"if":   `msg.meta.opcua_node_id == "ns=4;i=4"`,
								"then": `msg.meta.data_contract = "_count-model_v1";`,
							},
							map[string]any{
								"if":   `msg.meta.opcua_node_id == "ns=4;i=8"`,
								"then": `msg.meta.data_contract = "_historian";`,
							},
						},
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(Equal([]string{"_count-model_v1", "_historian"}))
	})

	It("extracts contracts from advancedProcessing", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"tag_processor": map[string]any{
						"advancedProcessing": `msg.meta.data_contract = "_pump_v1";`,
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(Equal([]string{"_pump_v1"}))
	})

	It("dedupes repeated contracts", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"tag_processor": map[string]any{
						"defaults": `msg.meta.data_contract = "_historian";
msg.meta.data_contract = "_historian";`,
						"conditions": []any{
							map[string]any{
								"then": `msg.meta.data_contract = "_historian";`,
							},
						},
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(Equal([]string{"_historian"}))
	})

	It("combines contracts across defaults, conditions, and advancedProcessing in first-seen order", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"tag_processor": map[string]any{
						"defaults": `msg.meta.data_contract = "_raw";`,
						"conditions": []any{
							map[string]any{
								"then": `msg.meta.data_contract = "_count-model_v1";`,
							},
						},
						"advancedProcessing": `msg.meta.data_contract = "_historian";`,
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(Equal([]string{"_raw", "_count-model_v1", "_historian"}))
	})

	It("skips processors that are not tag_processor", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"bloblang": `root = this`,
				},
				map[string]any{
					"tag_processor": map[string]any{
						"defaults": `msg.meta.data_contract = "_only_one";`,
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(Equal([]string{"_only_one"}))
	})

	It("returns nil when no tag_processor assigns the contract", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"tag_processor": map[string]any{
						"defaults": `msg.meta.tag_name = "foo";`,
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(BeNil())
	})

	It("tolerates whitespace around the assignment", func() {
		pipeline := map[string]any{
			"processors": []any{
				map[string]any{
					"tag_processor": map[string]any{
						"defaults": `msg.meta.data_contract   =   "_raw";`,
					},
				},
			},
		}
		Expect(extractDataContractsFromPipeline(pipeline)).To(Equal([]string{"_raw"}))
	})
})
