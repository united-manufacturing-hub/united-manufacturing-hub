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
