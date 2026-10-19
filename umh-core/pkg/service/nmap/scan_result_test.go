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

package nmap

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NmapScanResult.DescribeDialFailure", func() {
	It("reports no failure when the scan recorded none", func() {
		scan := NmapScanResult{PortResult: PortResult{State: string(PortStateOpen)}}

		_, failed := scan.DescribeDialFailure()
		Expect(failed).To(BeFalse())
	})

	It("names the failure class and the lookup and connect times", func() {
		scan := NmapScanResult{
			DialFailure: "dns_timeout",
			Metrics:     ScanMetrics{ResolveMs: 2201.4, ConnectMs: 0},
		}

		description, failed := scan.DescribeDialFailure()
		Expect(failed).To(BeTrue())
		Expect(description).To(ContainSubstring(scan.DialFailure))
		Expect(description).To(ContainSubstring("2201 ms"))
		Expect(description).To(ContainSubstring("0 ms"))
	})
})
