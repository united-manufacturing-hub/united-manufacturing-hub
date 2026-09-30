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

// The fsmv2 CPU path runs whenever USE_FSMV2_CPU is on. Management Console
// credentials are not a prerequisite: the fsmv2 supervisor runs without them.
var _ = Describe("FSMv2CPUEnabled, the fsmv2 CPU adoption flag", func() {
	DescribeTable("reports the effective state of the fsmv2 CPU path",
		func(flag, expected bool) {
			Expect(models.FSMv2CPUEnabled(flag)).To(Equal(expected))
		},
		Entry("flag on", true, true),
		Entry("flag off", false, false),
	)
})
