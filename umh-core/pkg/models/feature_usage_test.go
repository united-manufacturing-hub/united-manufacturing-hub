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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("FeatureUsage", func() {
	It("marshals all fields to correct JSON", func() {
		now := time.Now().UTC().Truncate(time.Second)
		fu := models.FeatureUsage{
			ConfigBackupEnabledSince: &now,
			ConfigBackupEnabled:      true,
			FSMv2Transport:           true,
			FSMv2MemoryCleanup:       true,
			FSMv2ProtocolConverter:   true,
			ResourceLimitBlocking:    true,
		}

		data, err := json.Marshal(fu)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]interface{}
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		Expect(raw).To(HaveKey("configBackupEnabled"))
		Expect(raw).To(HaveKey("configBackupEnabledSince"))
		Expect(raw).To(HaveKey("fsmv2Transport"))
		Expect(raw).To(HaveKey("fsmv2MemoryCleanup"))
		Expect(raw).To(HaveKey("fsmv2ProtocolConverter"))
		Expect(raw).To(HaveKey("resourceLimitBlocking"))
	})

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

	It("omits configBackupEnabledSince when nil", func() {
		fu := models.FeatureUsage{
			ConfigBackupEnabledSince: nil,
			ConfigBackupEnabled:      true,
		}

		data, err := json.Marshal(fu)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]interface{}
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		Expect(raw).NotTo(HaveKey("configBackupEnabledSince"))
	})
})
