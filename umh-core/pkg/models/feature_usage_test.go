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

	// The Console hides its delete control unless this arrives. Forgetting to send it
	// would look exactly like an older instance -- the control stays disabled and
	// nothing reports why -- so the key and its spelling are asserted directly.
	It("serializes the data contract delete guard capability", func() {
		usage := models.FeatureUsage{DataContractDeleteGuardEnabled: true}

		data, err := json.Marshal(usage)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]interface{}
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		Expect(raw).To(HaveKeyWithValue("dataContractDeleteGuardEnabled", true))
	})

	It("reads as absent on an instance that does not report it", func() {
		// An older instance omits the field entirely; the Console must read that as
		// "no guard" rather than as a default of true.
		var usage models.FeatureUsage
		Expect(json.Unmarshal([]byte(`{"historianConfigured":true}`), &usage)).To(Succeed())
		Expect(usage.DataContractDeleteGuardEnabled).To(BeFalse())
	})
})
