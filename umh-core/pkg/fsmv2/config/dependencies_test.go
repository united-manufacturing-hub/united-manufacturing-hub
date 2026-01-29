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

var _ = Describe("MergeDependencies", func() {
	It("returns nil when both are nil", func() {
		result := config.MergeDependencies(nil, nil)
		Expect(result).To(BeNil())
	})

	It("returns shallow copy of parent when child is nil", func() {
		parent := map[string]any{"key": "value", "num": 42}
		result := config.MergeDependencies(parent, nil)

		Expect(result).To(HaveKeyWithValue("key", "value"))
		Expect(result).To(HaveKeyWithValue("num", 42))
		Expect(result).To(HaveLen(2))

		// Verify it's a copy, not the same reference
		result["new"] = "should not affect parent"
		Expect(parent).ToNot(HaveKey("new"))
	})

	It("returns shallow copy of child when parent is nil", func() {
		child := map[string]any{"key": "value", "num": 42}
		result := config.MergeDependencies(nil, child)

		Expect(result).To(HaveKeyWithValue("key", "value"))
		Expect(result).To(HaveKeyWithValue("num", 42))
		Expect(result).To(HaveLen(2))

		// Verify it's a copy, not the same reference
		result["new"] = "should not affect child"
		Expect(child).ToNot(HaveKey("new"))
	})

	It("child overrides parent with same key", func() {
		parent := map[string]any{"key": "parent", "other": 1}
		child := map[string]any{"key": "child"}
		result := config.MergeDependencies(parent, child)

		Expect(result).To(HaveKeyWithValue("key", "child"))
		Expect(result).To(HaveKeyWithValue("other", 1))
		Expect(result).To(HaveLen(2))
	})

	It("combines unique keys from both", func() {
		parent := map[string]any{"parentKey": "pval"}
		child := map[string]any{"childKey": "cval"}
		result := config.MergeDependencies(parent, child)

		Expect(result).To(HaveLen(2))
		Expect(result).To(HaveKeyWithValue("parentKey", "pval"))
		Expect(result).To(HaveKeyWithValue("childKey", "cval"))
	})

	It("preserves interface values (shallow copy)", func() {
		ch := make(chan int)
		parent := map[string]any{"channel": ch}
		child := map[string]any{"other": "value"}

		result := config.MergeDependencies(parent, child)

		// Same channel instance, not a copy
		Expect(result["channel"]).To(BeIdenticalTo(ch))
	})

	It("allows child to set nil values (does not remove key)", func() {
		parent := map[string]any{"key": "value"}
		child := map[string]any{"key": nil}

		result := config.MergeDependencies(parent, child)

		Expect(result).To(HaveKey("key"))
		Expect(result["key"]).To(BeNil())
	})

	It("does not modify original parent map", func() {
		parent := map[string]any{"key": "parent"}
		child := map[string]any{"key": "child"}

		result := config.MergeDependencies(parent, child)

		Expect(parent["key"]).To(Equal("parent")) // Unchanged
		Expect(result["key"]).To(Equal("child"))
	})

	It("does not modify original child map", func() {
		parent := map[string]any{"parentKey": "pval"}
		child := map[string]any{"childKey": "cval"}

		result := config.MergeDependencies(parent, child)

		// Modify result
		result["newKey"] = "newVal"

		// Originals should be unchanged
		Expect(parent).ToNot(HaveKey("newKey"))
		Expect(child).ToNot(HaveKey("newKey"))
	})

	It("handles empty maps correctly", func() {
		parent := map[string]any{}
		child := map[string]any{"key": "value"}

		result := config.MergeDependencies(parent, child)

		Expect(result).To(HaveLen(1))
		Expect(result).To(HaveKeyWithValue("key", "value"))
	})

	It("handles both empty maps", func() {
		parent := map[string]any{}
		child := map[string]any{}

		result := config.MergeDependencies(parent, child)

		Expect(result).ToNot(BeNil())
		Expect(result).To(BeEmpty())
	})

	It("handles complex nested values (shallow copy verified by mutation)", func() {
		nested := map[string]any{"inner": "value"}
		parent := map[string]any{"nested": nested}
		child := map[string]any{"other": "data"}

		result := config.MergeDependencies(parent, child)

		// Verify shallow copy by mutating original and checking result reflects change
		nested["inner"] = "mutated"
		resultNested, ok := result["nested"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(resultNested["inner"]).To(Equal("mutated"))
	})

	It("handles multiple overrides correctly", func() {
		parent := map[string]any{
			"a": 1,
			"b": 2,
			"c": 3,
		}
		child := map[string]any{
			"b": 20,
			"c": 30,
			"d": 40,
		}

		result := config.MergeDependencies(parent, child)

		Expect(result).To(HaveLen(4))
		Expect(result).To(HaveKeyWithValue("a", 1))
		Expect(result).To(HaveKeyWithValue("b", 20))
		Expect(result).To(HaveKeyWithValue("c", 30))
		Expect(result).To(HaveKeyWithValue("d", 40))
	})
})
