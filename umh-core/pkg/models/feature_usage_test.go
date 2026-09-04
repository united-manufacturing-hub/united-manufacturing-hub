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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("FeatureUsage", func() {
	It("omits featureUsage from Core JSON when nil", func() {
		core := models.Core{
			FeatureUsage: nil,
		}

		data, err := json.Marshal(core)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]interface{}
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		Expect(raw).NotTo(HaveKey("featureUsage"))
	})

	It("serializes the FSMv2 CPU flag under the JSON key fsmv2CpuEnabled", func() {
		usage := models.FeatureUsage{
			FSMv2CPUEnabled: true,
		}

		data, err := json.Marshal(usage)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]interface{}
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		Expect(raw).To(HaveKeyWithValue("fsmv2CpuEnabled", true))
	})

	It("serializes the historian adoption fields", func() {
		usage := models.FeatureUsage{
			HistorianConfigured:  true,
			HistorianBridgeCount: 3,
		}

		data, err := json.Marshal(usage)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]interface{}
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		Expect(raw).To(HaveKeyWithValue("historianConfigured", true))
		Expect(raw).To(HaveKeyWithValue("historianBridgeCount", float64(3)))
	})
})

// An instance that fell back to the legacy CPU path never counts as enabled:
// the flag and every prerequisite must hold. models.FSMv2CPUEnabled names the
// prerequisites.
var _ = Describe("FSMv2CPUEnabled, the fsmv2 CPU adoption flag", func() {
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
