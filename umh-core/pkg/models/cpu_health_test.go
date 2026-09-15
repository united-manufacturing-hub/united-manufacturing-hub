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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("CPU", func() {
	It("marshals cpuHealth's verdict and every Details member inline at the same level, and omits the cpuHealth key when CPUHealth is nil", func() {
		// The console's adapter is built against these exact key names: a key
		// renamed here reads on its schema as absent-and-invalid, which nothing
		// downstream distinguishes from a legitimately-absent optional.
		detailKeys := []string{
			"p95UsageCores",
			"throttleRatio",
			"pressureAvg60",
			"stealP95",
			"avgUsageFraction",
			"avgUsageCores",
			"hostHeadroomCores",
			"avgHostBusyCores",
			"capacityCores",
			"reserveCores",
			"logicalCpus",
			"hostCpus",
			"usageRingActive",
			"hostBusyRingActive",
			"limitedVisibility",
			"hostBusyCoresAvailable",
			"limitApplies",
			"pressureApplies",
			"stealApplies",
			"hostHeadroomAvailable",
			"throttleSignalReady",
			"pressureSignalReady",
			"stealSignalReady",
		}

		cpu := models.CPU{
			CPUHealth: &models.CPUHealth{
				Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
				Details: cpuhealth.Details{
					LogicalCpus: 8,
					HostCpus:    4,
				},
			},
		}

		data, err := json.Marshal(cpu)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]interface{}
		Expect(json.Unmarshal(data, &raw)).To(Succeed())
		Expect(raw).To(HaveKey("cpuHealth"))

		health, ok := raw["cpuHealth"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "cpuHealth must be a JSON object")

		// verdict sits BESIDE the Details members and no "details" object
		// wraps them, which is what separates embedding from the nested
		// shape pkg/fsmv2/cpu.CPUStatus deliberately uses.
		Expect(health).To(HaveKey("verdict"))
		Expect(health).NotTo(HaveKey("details"))
		Expect(health).To(HaveLen(len(detailKeys) + 1))
		for _, key := range detailKeys {
			Expect(health).To(HaveKey(key))
		}

		// The inline keys carry the Details values, not just the names.
		Expect(health["logicalCpus"]).To(Equal(float64(8)))
		Expect(health["hostCpus"]).To(Equal(float64(4)))

		verdict, ok := health["verdict"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(verdict["state"]).To(Equal("healthy"))

		dataNil, err := json.Marshal(models.CPU{CPUHealth: nil})
		Expect(err).NotTo(HaveOccurred())

		var rawNil map[string]interface{}
		Expect(json.Unmarshal(dataNil, &rawNil)).To(Succeed())
		Expect(rawNil).NotTo(HaveKey("cpuHealth"))
	})
})
