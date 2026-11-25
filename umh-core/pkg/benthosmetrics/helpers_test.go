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

// Helpers shared by the package's tests: a port to observe and a Metrics
// body.

package benthosmetrics

import (
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// testPort is the port the tests scrape; only its value matters, never the
// endpoint behind it.
const testPort = 4195

// counts returns a Metrics whose input and output totals are input and
// output.
func counts(input, output int) Metrics {
	return Metrics{
		Inputs:  map[string]InputMetrics{"root.input": {Received: int64(input)}},
		Outputs: map[string]OutputMetrics{"root.output": {Sent: int64(output)}},
	}
}

// perSecond returns a measured throughput, and fails the spec when it is
// Unknown.
func perSecond(r diagnosis.Reading) float64 {
	v, ok := r.Get()
	ExpectWithOffset(1, ok).To(BeTrue(), "the throughput must be measured")

	return v
}

// unmeasured is the throughput of a window that cannot answer yet.
var unmeasured = diagnosis.Unknown()

// failuresIn returns how many of f's fields hold a failure. It lists the
// fields itself rather than reading errorCounters and connectionChecks, so a
// field missing from those tables still counts.
func failuresIn(f FailureReport) int {
	n := 0

	for _, set := range []bool{
		f.OutputErrors != nil,
		f.ProcessorErrors != nil,
		f.InputsNotConnected != nil,
		f.OutputsNotConnected != nil,
	} {
		if set {
			n++
		}
	}

	return n
}
