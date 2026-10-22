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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("DeepCloneMap", func() {
	It("copies a nested map with non-string keys, as yaml decodes an integer-keyed mapping", func() {
		src := map[string]any{
			"location": map[any]any{1: "hall"},
			"list":     []any{map[any]any{"k": "v"}},
		}

		clone := config.DeepCloneMap(src)
		clone["location"].(map[any]any)[1] = "changed"
		clone["list"].([]any)[0].(map[any]any)["k"] = "changed"

		Expect(src["location"]).To(Equal(map[any]any{1: "hall"}))
		Expect(src["list"]).To(Equal([]any{map[any]any{"k": "v"}}))
	})
})
