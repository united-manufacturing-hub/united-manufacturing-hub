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

package variables_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"gopkg.in/yaml.v3"
)

var _ = Describe("VariableBundle User Experience Examples", func() {
	It("should demonstrate user-friendly YAML marshaling", func() {
		// Create a bundle with all namespaces (as would happen internally)
		vb := variables.VariableBundle{
			User: map[string]any{
				"HOST":    "wttr.in",
				"PORT":    "8080",
				"TIMEOUT": 30,
			},
			Global: map[string]any{
				"release_channel": "stable",
				"cluster_id":      "prod-cluster-1",
			},
			Internal: map[string]any{
				"id":         "protocol-converter-123",
				"bridged_by": "protocol-converter-node1-pc123",
			},
		}

		// When marshaling for user consumption (e.g., config export)
		userYaml, err := yaml.Marshal(&vb)
		Expect(err).NotTo(HaveOccurred())

		fmt.Printf("User-facing YAML:\n%s\n", string(userYaml))

		// Should only contain user variables at top level
		Expect(string(userYaml)).To(ContainSubstring("HOST: wttr.in"))
		Expect(string(userYaml)).To(ContainSubstring("PORT: \"8080\""))
		Expect(string(userYaml)).To(ContainSubstring("TIMEOUT: 30"))

		// Should NOT contain internal namespaces
		Expect(string(userYaml)).NotTo(ContainSubstring("global:"))
		Expect(string(userYaml)).NotTo(ContainSubstring("internal:"))
		Expect(string(userYaml)).NotTo(ContainSubstring("user:"))

		// When using the generator for internal/debugging purposes
		generator := variables.NewGenerator()
		internalYaml, err := generator.RenderConfig(vb)
		Expect(err).NotTo(HaveOccurred())

		fmt.Printf("Internal/debugging YAML:\n%s\n", string(internalYaml))

		// Should contain all namespaces
		Expect(internalYaml).To(ContainSubstring("user:"))
		Expect(internalYaml).To(ContainSubstring("global:"))
		Expect(internalYaml).To(ContainSubstring("internal:"))
		Expect(internalYaml).To(ContainSubstring("release_channel: stable"))
		Expect(internalYaml).To(ContainSubstring("bridged_by: protocol-converter-node1-pc123"))
	})

	It("should demonstrate user-friendly YAML unmarshaling", func() {
		// User writes this simple config
		userConfig := `
HOST: wttr.in
PORT: "8080"
TIMEOUT: 30
nested:
  database_url: "postgres://wttr.in:5432/mydb"
  retries: 3
`

		// System unmarshals it
		var vb variables.VariableBundle
		err := yaml.Unmarshal([]byte(userConfig), &vb)
		Expect(err).NotTo(HaveOccurred())

		// All variables end up in User namespace automatically
		Expect(vb.User).To(HaveKeyWithValue("HOST", "wttr.in"))
		Expect(vb.User).To(HaveKeyWithValue("PORT", "8080"))
		Expect(vb.User).To(HaveKeyWithValue("TIMEOUT", 30))
		Expect(vb.User).To(HaveKey("nested"))

		// System can then add global and internal variables programmatically
		vb.Global = map[string]any{
			"cluster_id": "prod-cluster-1",
		}
		vb.Internal = map[string]any{
			"id":         "pc-123",
			"bridged_by": "protocol-converter-node1-pc123",
		}

		// Template rendering works as expected
		scope := vb.Flatten()
		Expect(scope).To(HaveKeyWithValue("HOST", "wttr.in"))
		Expect(scope).To(HaveKey("global"))
		Expect(scope).To(HaveKey("internal"))

		globalScope, ok := scope["global"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(globalScope).To(HaveKeyWithValue("cluster_id", "prod-cluster-1"))

		internalScope, ok := scope["internal"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(internalScope).To(HaveKeyWithValue("bridged_by", "protocol-converter-node1-pc123"))

		fmt.Printf("Final flattened scope for template rendering:\n")
		for k, v := range scope {
			fmt.Printf("  %s: %v\n", k, v)
		}
	})
})
