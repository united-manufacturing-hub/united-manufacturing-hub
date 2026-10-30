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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/memoryhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var memoryHealthDetailKeys = []string{
	"avgUsedBytes",
	"avgUsedFraction",
	"usedRingActive",
	"limitApplies",
	"totalBytes",
	"avgLimitUsedFraction",
	"limitRingActive",
	"hostTotalBytes",
	"avgHostUsedFraction",
	"hostRingActive",
	"pressureAvg60",
	"pressureApplies",
	"pressureSignalReady",
	"oomKillsLast60s",
	"oomKillsApply",
	"oomSignalReady",
}

func marshalToMap(value any) map[string]any {
	data, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())

	var raw map[string]any
	Expect(json.Unmarshal(data, &raw)).To(Succeed())

	return raw
}

var _ = Describe("Memory", func() {
	It("marshals memoryHealth's verdict and every Details member inline at the same level", func() {
		memory := models.Memory{
			MemoryHealth: &models.MemoryHealth{
				Verdict: memoryhealth.Verdict{
					State:       memoryhealth.StateDegraded,
					Attribution: memoryhealth.AttributionHost,
					Causes: []memoryhealth.Cause{{
						Kind:        memoryhealth.CauseKindHostMemoryFull,
						Instrument:  "host-used-fraction",
						Unit:        "fraction",
						Attribution: memoryhealth.AttributionHost,
						Value:       0.9,
					}},
				},
				Details: memoryhealth.Details{
					AvgHostUsedFraction: 0.9,
					HostRingActive:      true,
					HostTotalBytes:      16,
				},
			},
		}

		health, ok := marshalToMap(memory)["memoryHealth"].(map[string]any)
		Expect(ok).To(BeTrue(), "memoryHealth must be a JSON object")

		Expect(health).NotTo(HaveKey("details"))
		Expect(health).To(HaveLen(len(memoryHealthDetailKeys) + 1))
		for _, key := range memoryHealthDetailKeys {
			Expect(health).To(HaveKey(key))
		}

		Expect(health["avgHostUsedFraction"]).To(Equal(0.9))
		Expect(health["hostRingActive"]).To(Equal(true))
		Expect(health["pressureSignalReady"]).To(Equal(false))
		Expect(health["hostTotalBytes"]).To(Equal(float64(16)))

		verdict, ok := health["verdict"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(verdict["state"]).To(Equal("degraded"))
		Expect(verdict["attribution"]).To(Equal("host"))
		Expect(verdict["causes"]).To(ConsistOf(map[string]any{
			"kind":        "host-memory-full",
			"instrument":  "host-used-fraction",
			"unit":        "fraction",
			"attribution": "host",
			"value":       0.9,
		}))
	})

	It("omits the memoryHealth key when MemoryHealth is nil", func() {
		Expect(marshalToMap(models.Memory{})).NotTo(HaveKey("memoryHealth"))
	})
})
