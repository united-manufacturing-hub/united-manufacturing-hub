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

package communicator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fsmconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// Variables-merge parity: communicator's RenderChildren uses BaseUserSpec{}
// (empty Variables); the previous DeriveDesiredState passed the parent's full
// UserSpec. After supervisor merges parent + child, both produce the same set
// because Merge(parent, parent) == Merge(parent, {}) == parent. This test
// pins that invariant so a future Merge regression is caught.
var _ = Describe("communicator Variables-merge invariant", func() {
	It("Merge(parentVars, parentVars) equals Merge(parentVars, {}) for all User keys", func() {
		parentVars := fsmconfig.VariableBundle{
			User: map[string]any{
				"RELAY_HOST": "relay.example.com",
				"PORT":       "8080",
				"TMPL":       "{{ .RELAY_HOST }}/api",
			},
		}

		legacyResult := fsmconfig.Merge(parentVars, parentVars)
		newResult := fsmconfig.Merge(parentVars, fsmconfig.VariableBundle{})

		Expect(legacyResult.User).To(Equal(newResult.User),
			"both paths must yield identical User variables after supervisor merge")
	})

	It("template syntax strings are preserved unchanged through merge", func() {
		parentVars := fsmconfig.VariableBundle{
			User: map[string]any{
				"RELAY_URL": "https://{{ .HOST }}:{{ .PORT }}",
			},
		}

		result := fsmconfig.Merge(parentVars, fsmconfig.VariableBundle{})

		Expect(result.User["RELAY_URL"]).To(Equal("https://{{ .HOST }}:{{ .PORT }}"),
			"template syntax must not be evaluated or mangled by Merge")
	})
})
