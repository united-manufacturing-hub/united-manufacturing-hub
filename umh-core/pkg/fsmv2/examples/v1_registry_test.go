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
	"maps"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

// Remove a name here when its scenario moves to RegistryV2 (ENG-5114).
var frozenV1ScenarioNames = []string{
	"simple", "cascade",
	"configerror", "inheritance", "communicator", "persistence",
}

var _ = Describe("the v1 scenario registry", func() {
	It("holds exactly the frozen v1 names", func() {
		Expect(slices.Collect(maps.Keys(examples.Registry))).To(ConsistOf(frozenV1ScenarioNames),
			"the v1 registry changed: write new scenarios as a ScenarioV2 in RegistryV2 (see the \"Writing a scenario\" section of pkg/fsmv2/CLAUDE.md), and remove a name here when its scenario moves to v2")
	})
})
