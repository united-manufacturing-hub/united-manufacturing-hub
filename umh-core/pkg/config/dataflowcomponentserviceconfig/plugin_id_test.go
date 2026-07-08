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

var _ = Describe("BenthosPluginID", func() {
	DescribeTable("extracts the benthos plugin name",
		func(input map[string]any, expected string) {
			Expect(dataflowcomponentserviceconfig.BenthosPluginID(input)).To(Equal(expected))
		},
		Entry("nil map", nil, ""),
		Entry("empty map", map[string]any{}, ""),
		Entry("direct single key", map[string]any{"mqtt": map[string]any{"topic": "t"}}, "mqtt"),
		Entry("wrapped single key (normal observed form)", map[string]any{"input": map[string]any{"http_client": map[string]any{"url": "u"}}}, "http_client"),
		Entry("wrapped empty inner", map[string]any{"input": map[string]any{}}, ""),
		Entry("wrapped scalar inner", map[string]any{"input": "scalar"}, ""),
		Entry("labeled direct form", map[string]any{"label": "x", "http_client": map[string]any{}}, "http_client"),
		Entry("labeled wrapped form", map[string]any{"input": map[string]any{"label": "x", "http_client": map[string]any{}}}, "http_client"),
		Entry("two top-level plugin keys", map[string]any{"mqtt": map[string]any{}, "http_client": map[string]any{}}, ""),
		Entry("two plugin keys inside input wrapper", map[string]any{"input": map[string]any{"mqtt": map[string]any{}, "http_client": map[string]any{}}}, ""),
		Entry("processors metadata + direct plugin", map[string]any{"processors": []any{}, "mqtt": map[string]any{}}, "mqtt"),
		Entry("processors metadata inside input wrapper", map[string]any{"input": map[string]any{"processors": []any{}, "mqtt": map[string]any{}}}, "mqtt"),
		Entry("scalar value direct", map[string]any{"mqtt": "scalar"}, "mqtt"),
	)
})
