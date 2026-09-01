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

package models_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// FSMv2CPUEnabled reports the flag's EFFECTIVE state, not merely whether
// USE_FSMV2_CPU is set: the flag and every prerequisite the seam needs must be
// present, or the field reports false and an instance that fell back to legacy
// never counts as enabled.
var _ = Describe("FSMv2CPUEnabled", func() {
	DescribeTable("reports the effective state of the fsmv2 CPU path",
		func(flag, transport, apiURLSet, authTokenSet, expected bool) {
			Expect(models.FSMv2CPUEnabled(flag, transport, apiURLSet, authTokenSet)).
				To(Equal(expected))
		},
		Entry("flag on, transport on, API_URL and AUTH_TOKEN present", true, true, true, true, true),
		Entry("flag on but transport off (the rollout inflation case)", true, false, true, true, false),
		Entry("flag on, transport on, API_URL missing", true, true, false, true, false),
		Entry("flag on, transport on, AUTH_TOKEN missing", true, true, true, false, false),
		Entry("flag off even with every prerequisite present", false, true, true, true, false),
	)
})