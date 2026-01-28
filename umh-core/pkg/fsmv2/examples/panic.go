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

// PanicScenario demonstrates panic recovery in action handlers.
//
// ActionExecutor catches panics via defer/recover, logs with stack trace, and
// allows retry on next tick. Worker stays in TryingToConnect, never reaching Connected.
var PanicScenario = Scenario{
	Name: "panic",

	Description: "Demonstrates panic recovery in action handlers",

	YAMLConfig: `
children:
  - name: "panic-worker-1"
    workerType: "examplepanic"
    userSpec:
      config: |
        should_panic: true
`,
}
