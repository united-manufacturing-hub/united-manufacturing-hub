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

package examples

// HelloworldScenario runs a single helloworld worker.
//
// Demonstrates both I/O patterns:
//   - Action: SayHelloAction writes to in-memory deps
//   - Observation: CollectObservedState reads /tmp/helloworld-mood from disk
//
// Interactive demo:
//
//	echo "happy" > /tmp/helloworld-mood   # stays Running
//	echo "sad" > /tmp/helloworld-mood     # → Degraded
//	rm /tmp/helloworld-mood               # → Running
var HelloworldScenario = Scenario{
	Name:        "helloworld",
	Description: "Minimal worker: says hello, reads mood from /tmp/helloworld-mood",
	YAMLConfig: `
children:
  - name: "hello-1"
    workerType: "helloworld"
    userSpec:
      config: |
        state: running
`,
}
