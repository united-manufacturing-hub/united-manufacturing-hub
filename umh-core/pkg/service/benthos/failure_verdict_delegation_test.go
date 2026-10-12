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

package benthos

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

var _ = Describe("IsMetricsErrorFree with a failure verdict", func() {
	var service *BenthosService

	BeforeEach(func() {
		service = &BenthosService{}
	})

	// nineLifetimeErrors has 9 output errors in its cumulative counters.
	nineLifetimeErrors := benthos_monitor.Metrics{
		Outputs: map[string]benthos_monitor.OutputInstance{"root.output": {Error: 9}},
	}

	It("returns the verdict's result when the verdict has a fired entry", func() {
		verdict := &benthos_monitor.FailureVerdict{
			Fired: []benthos_monitor.FailureEntry{{
				Signal:   "output_error",
				Label:    "output errors",
				Increase: 2,
				Since:    time.Unix(1_800_000_000, 0),
				Window:   constants.BenthosLogWindow,
			}},
		}
		wantOK, wantReason := verdict.ErrorFree()

		ok, reason := service.IsMetricsErrorFree(benthos_monitor.BenthosMetrics{Failures: verdict})
		Expect(ok).To(Equal(wantOK))
		Expect(reason).To(Equal(wantReason))
		Expect(reason).To(Equal("benthos reported 2 output errors in the last 10m0s"))
	})

	It("is error-free with an empty verdict even though the cumulative counters hold 9 output errors", func() {
		ok, reason := service.IsMetricsErrorFree(benthos_monitor.BenthosMetrics{
			Metrics:  nineLifetimeErrors,
			Failures: &benthos_monitor.FailureVerdict{},
		})
		Expect(ok).To(BeTrue())
		Expect(reason).To(BeEmpty())
	})

	It("reads the cumulative counters when there is no verdict", func() {
		ok, reason := service.IsMetricsErrorFree(benthos_monitor.BenthosMetrics{Metrics: nineLifetimeErrors})
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reported 9 output errors"))
	})
})
