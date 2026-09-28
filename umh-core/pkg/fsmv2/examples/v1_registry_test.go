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

package examples_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

// v1ScenarioNames is the frozen list of the v1 registry's names. Names leave
// it as their scenarios move to v2 (ENG-5114), and none are added.
var v1ScenarioNames = []string{
	"simple", "failing", "panic", "slow", "cascade", "timeout",
	"configerror", "inheritance", "communicator", "concurrent", "persistence",
}

var _ = Describe("the v1 scenario registry", func() {
	It("does not grow the v1 scenario registry", func() {
		for name := range examples.Registry {
			Expect(v1ScenarioNames).To(ContainElement(name),
				"the v1 registry gained %q: write new scenarios as a ScenarioV2 in RegistryV2; see the \"Writing a scenario\" section of pkg/fsmv2/CLAUDE.md", name)
		}
	})
})
