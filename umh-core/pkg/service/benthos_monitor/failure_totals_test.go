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

package benthos_monitor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("failure counter totals", func() {
	It("sums each failure counter across every path", func() {
		m := Metrics{
			Inputs: map[string]InputInstance{
				"root.input.broker.inputs.0": {ConnectionFailed: 2},
				"root.input.broker.inputs.1": {ConnectionFailed: 3},
			},
			Outputs: map[string]OutputInstance{
				"root.output.switch.cases.0": {ConnectionFailed: 4},
				"root.output.switch.cases.1": {ConnectionFailed: 5},
			},
			Process: ProcessMetrics{Processors: map[string]ProcessorMetrics{
				"root.pipeline.processors.0": {Error: 6},
				"root.pipeline.processors.1": {Error: 7},
			}},
		}

		Expect(m.InputConnectionFailedTotal()).To(Equal(int64(2 + 3)))
		Expect(m.OutputConnectionFailedTotal()).To(Equal(int64(4 + 5)))
		Expect(m.ProcessorErrorTotal()).To(Equal(int64(6 + 7)))
	})
})
