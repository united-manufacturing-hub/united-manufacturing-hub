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

package memoryhealth

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var (
	degradedVerdict = Verdict{State: StateDegraded, Causes: []Cause{{Kind: CauseKindLimitFull, Value: 0.9}}}
	healthyVerdict  = Verdict{State: StateHealthy}
)

func detailsAt(usedFraction float64) Details {
	return Details{UsedFraction: diagnosis.Known(usedFraction)}
}

var _ = Describe("ComposeMessage", func() {
	DescribeTable("names the usage level",
		func(verdict Verdict, details Details, expectedMessage string) {
			Expect(ComposeMessage(verdict, details)).To(Equal(expectedMessage))
		},
		Entry("a healthy verdict below the warning level", healthyVerdict, detailsAt(0.5), messageNormal),
		Entry("a healthy verdict at exactly the warning level", healthyVerdict, detailsAt(warningFraction), messageWarning),
		Entry("a degraded verdict", degradedVerdict, detailsAt(0.9), messageCritical),
		Entry("a degraded verdict below the degraded level before it recovers", degradedVerdict, detailsAt(0.77), messageCritical),
		Entry("an unmeasured window", healthyVerdict, Details{UsedFraction: diagnosis.Unknown()}, messageNotMeasured),
	)
})
