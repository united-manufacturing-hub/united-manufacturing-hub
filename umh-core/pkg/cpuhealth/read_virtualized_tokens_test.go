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

package cpuhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the DMI vendor hypervisor token list", func() {
	It("must never gain \"microsoft\": sys_vendor \"Microsoft Corporation\" cannot tell an Azure ARM guest from a bare-metal Surface (ENG-5642)", func() {
		Expect(dmiVendorHypervisorTokens).NotTo(ContainElement("microsoft"),
			"adding \"microsoft\" to dmiVendorHypervisorTokens would misread every bare-metal Microsoft Surface as a VM, because sys_vendor \"Microsoft Corporation\" cannot distinguish it from an Azure ARM guest — the worse error. Azure ARM stays a documented known limitation (ENG-5642) instead.")
	})
})
