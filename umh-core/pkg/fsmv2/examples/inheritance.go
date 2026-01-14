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
// # What This Scenario Tests
//
// Variable inheritance is a key FSM v2 feature that allows parent workers to pass
// configuration values down to their children. This scenario tests the end-to-end flow:
//
//  1. User Configuration (config.yaml)
//     - Parent worker defines variables in userSpec.variables.user
//     - Example: IP="192.168.1.100", PORT=502
//
//  2. Child Creation (DeriveDesiredState → GetChildSpecs)
//     - Parent's DeriveDesiredState() returns ChildrenSpecs
//     - Each ChildSpec can define its own variables (e.g., DEVICE_ID="plc-01")
//     - These child variables override parent variables with same key
//
//  3. Variable Merge (reconcileChildren)
//     - Supervisor calls config.Merge(parent.Variables, child.Variables)
//     - Parent's User variables copied first
//     - Child's User variables override parent's
//     - Result: child has all parent vars plus its own
//
//  4. Template Expansion
//     - Child's userSpec.config can use {{ .IP }}, {{ .PORT }}, {{ .DEVICE_ID }}
//     - Variables are flattened: userSpec.variables.user.IP becomes {{ .IP }}
//
// # Hierarchy Created
//
//	inheritance-parent (exampleparent)
//	  ├── child-0 (examplechild) - inherits IP, PORT from parent
//	  └── child-1 (examplechild) - inherits IP, PORT from parent
//
// # Variable Flow
//
//	Parent Config:
//	  variables:
//	    user:
//	      IP: "192.168.1.100"
//	      PORT: 502
//
//	Child ChildSpec (created by parent):
//	  variables:
//	    user:
//	      DEVICE_ID: "plc-{{ index }}"  # Child-specific
//
//	After Merge (child ends up with):
//	  variables:
//	    user:
//	      IP: "192.168.1.100"           # Inherited from parent
//	      PORT: 502                      # Inherited from parent
//	      DEVICE_ID: "plc-0"             # Its own variable
//
// # How to Verify This Scenario
//
// When running this scenario, watch for:
//
// Phase 1: Startup
//   - Parent starts with IP and PORT variables
//   - Children are created
//
// Phase 2: Variable Propagation
//   - Look for "variables_propagated" in logs
//   - Verify children receive parent's variables
//
// Phase 3: Verification
//   - Test assertions verify children have both inherited and own variables
//   - Parent's IP/PORT are accessible in child's variable bundle
//   - Child's DEVICE_ID is preserved
//
// # Real-World Parallels
//
// Connection Pool to Device:
//   - Pool manager (parent) knows IP address and port of PLC
//   - Individual connections (children) need device-specific identifiers
//   - Children inherit connection details, add their own identifiers
//
// Protocol Converter:
//   - Bridge (parent) has IP="192.168.1.100", PORT=502
//   - Source flow (child) inherits IP, PORT, adds POLL_INTERVAL=1000
//   - Sink flow (child) inherits IP, PORT, adds BATCH_SIZE=100
//
// # Why This Matters for Production
//
// In production, variable inheritance enables:
//
//  1. Configuration Reuse: Define connection details once in parent
//  2. DRY Principle: No need to repeat IP/PORT in every child config
//  3. Hierarchical Config: Enterprise → Site → Area → Line → Cell inheritance
//  4. Template Simplicity: Children use {{ .IP }} without knowing source
//
// Without variable inheritance, every child would need full configuration,
// leading to duplication and potential inconsistencies when connection
// details change.
var InheritanceScenario = Scenario{
	Name: "inheritance",

	Description: "Demonstrates variable inheritance: parent passes IP/PORT to children, children add DEVICE_ID",

	YAMLConfig: `
children:
  # Parent with connection variables that children will inherit
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
