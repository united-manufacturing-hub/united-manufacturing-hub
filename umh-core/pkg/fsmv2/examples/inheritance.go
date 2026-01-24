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

// InheritanceScenario demonstrates variable inheritance from parent to child workers.
//
// Parent defines variables (IP, PORT) in userSpec.variables.user. Children created
// via GetChildSpecs can define their own variables (DEVICE_ID) which override parent
// values with same key. Variables are flattened for templates: {{ .IP }}, {{ .PORT }}.
var InheritanceScenario = Scenario{
	Name: "inheritance",

	Description: "Demonstrates variable inheritance: parent passes IP/PORT to children, children add DEVICE_ID",

	YAMLConfig: `
children:
  # Parent with connection variables for child inheritance
  - name: "inheritance-parent"
    workerType: "exampleparent"
    userSpec:
      config: |
        children_count: 2
      variables:
        user:
          IP: "192.168.1.100"
          PORT: 502
          CONNECTION_NAME: "factory-plc"
`,
}
