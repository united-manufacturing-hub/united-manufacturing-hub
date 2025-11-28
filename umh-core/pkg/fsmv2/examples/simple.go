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

// This file defines the SimpleScenario, which serves as both a working example
// and comprehensive documentation for creating FSM v2 test scenarios.
//
// # What is a Scenario?
//
// A Scenario is a complete test configuration that defines:
//   - Name: CLI identifier (e.g., --scenario=simple)
//   - Description: Human-readable explanation of what this scenario tests
//   - YAMLConfig: The application configuration that creates the worker hierarchy
//
// Scenarios are used by:
//   - Integration tests (with assertions on logs and state)
//   - CLI runner (for manual verification and debugging)
//   - Stress tests (future: high load, failure injection)
//
// # How the Scenario System Works
//
// ## 1. Scenario Definition (this file)
//
// Each scenario is a Scenario struct with YAML configuration. The YAML defines
// the initial worker hierarchy using the same format as production configurations.
//
// ## 2. Scenario Registry (scenario.go)
//
// All scenarios are registered in the Registry map, making them available to
// both tests and CLI. To add a new scenario:
//   - Create a new file (e.g., stress.go)
//   - Define your Scenario
//   - Add it to Registry in scenario.go
//
// ## 3. Test Usage
//
// Tests use scenarios via the Run function:
//
//	done, err := examples.Run(ctx, examples.RunConfig{
//	    Scenario:     examples.SimpleScenario,
//	    Duration:     10 * time.Second,
//	    TickInterval: 100 * time.Millisecond,
//	    Logger:       testLogger,  // Can be test logger for capture
//	    Store:        store,        // Can be spy store for assertions
//	})
//
// ## 4. CLI Usage
//
// The CLI runner uses the same scenarios for manual testing:
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=simple --duration=10s
//
// # Understanding YAMLConfig Structure
//
// The YAMLConfig follows the application worker configuration format:
//
//	children:
//	  - name: "worker-unique-id"
//	    workerType: "registered-worker-type"
//	    location:
//	      - enterprise: "value"
//	      - site: "value"
//	    userSpec:
//	      config: |
//	        key: value
//	      variables:
//	        user:
//	          KEY: "value"
//
// ## Key Fields Explained
//
// ### children (array, required)
// Defines the root-level workers. Each child is a ChildSpec that can have its own children,
// creating a hierarchical tree structure.
//
// ### name (string, required)
// Unique identifier for this worker within its parent's children. Must be unique among siblings.
// Used in logs, metrics, and for referencing the worker.
//
// ### workerType (string, required)
// The registered worker type. Must match a worker registered via RegisterWorkerType().
// Worker types are auto-registered via blank imports (see below).
//
// ### location (array, optional)
// ISA-95 hierarchy levels for this worker:
//   - enterprise: Top-level organization
//   - site: Physical location/facility
//   - area: Functional area within site
//   - line: Production line
//   - cell: Work cell
//
// Location is merged with parent's location and exposed as {{ .location_path }} in templates.
//
// ### userSpec (object, required)
// Worker-specific configuration passed to the worker's UserSpec parser.
//
// #### userSpec.config (string, required)
// YAML/JSON string passed to the worker. The worker parses this into its typed UserSpec struct.
// Can use template variables like {{ .KEY }}.
//
// #### userSpec.variables (object, optional)
// Variables available for template expansion. Three namespaces:
//
//   - user: User-defined variables, flattened to top level ({{ .KEY }})
//   - global: System-wide variables, nested ({{ .global.KEY }})
//   - internal: FSM-internal variables, nested ({{ .internal.KEY }})
//
// Variables are flattened before template rendering: userSpec.variables.user.KEY becomes {{ .KEY }}.
//
// # Worker Registration via Blank Imports
//
// Workers are automatically registered when their package is imported:
//
//	import (
//	    _ "github.com/.../fsmv2/workers/example/example-parent"
//	    _ "github.com/.../fsmv2/workers/example/example-child"
//	)
//
// Each worker package has an init() function that calls RegisterWorkerType():
//
//	func init() {
//	    factory.RegisterWorkerType("parent", &ParentWorker{})
//	}
//
// The blank import (_) ensures the package's init() runs, registering the worker
// without creating unused imports.
//
// # Creating Custom Test Workers
//
// To create a new worker for testing:
//
// ## 1. Define Worker Structure
//
//	package myworker
//
//	import (
//	    "github.com/.../fsmv2/worker"
//	    "github.com/.../fsmv2/factory"
//	)
//
//	type MyWorker struct {
//	    worker.BaseWorker
//	}
//
// ## 2. Register Worker Type
//
//	func init() {
//	    factory.RegisterWorkerType("myworker", &MyWorker{})
//	}
//
// ## 3. Define UserSpec
//
//	type MyWorkerUserSpec struct {
//	    Setting1 string `json:"setting1" yaml:"setting1"`
//	    Setting2 int    `json:"setting2" yaml:"setting2"`
//	}
//
// ## 4. Implement Worker Interface
//
// Workers must implement:
//   - ParseUserSpec(config string) (interface{}, error)
//   - GetChildSpecs(userSpec interface{}) ([]config.ChildSpec, error)
//   - Lifecycle methods (Start, Stop, etc.)
//
// ## 5. Use in Scenario
//
//	import _ "path/to/myworker"
//
//	var MyScenario = Scenario{
//	    Name: "my-scenario",
//	    YAMLConfig: `
//	children:
//	  - name: "my-worker-1"
//	    workerType: "myworker"
//	    userSpec:
//	      config: |
//	        setting1: "value"
//	        setting2: 42
//	`,
//	}
//
// # Store Injection for Assertions
//
// Tests can inject a spy store to assert on worker state:
//
//	type SpyStore struct {
//	    storage.TriangularStoreInterface
//	    insertedDocs []storage.Document
//	}
//
//	func (s *SpyStore) Insert(doc storage.Document) error {
//	    s.insertedDocs = append(s.insertedDocs, doc)
//	    return s.TriangularStoreInterface.Insert(doc)
//	}
//
//	spyStore := &SpyStore{TriangularStoreInterface: realStore}
//	done, err := examples.Run(ctx, examples.RunConfig{
//	    Scenario: examples.SimpleScenario,
//	    Store:    spyStore,
//	    // ...
//	})
//
//	// After scenario completes:
//	Expect(spyStore.insertedDocs).To(ContainElement(HaveField("ID", "parent-1")))
//
// # Logger Capture for Verification
//
// Tests can capture log output for assertions:
//
//	type LogCapture struct {
//	    *zap.SugaredLogger
//	    entries []string
//	}
//
//	func (l *LogCapture) Info(args ...interface{}) {
//	    l.entries = append(l.entries, fmt.Sprint(args...))
//	    l.SugaredLogger.Info(args...)
//	}
//
//	capture := &LogCapture{SugaredLogger: realLogger}
//	done, err := examples.Run(ctx, examples.RunConfig{
//	    Scenario: examples.SimpleScenario,
//	    Logger:   capture,
//	    // ...
//	})
//
//	// After scenario completes:
//	Expect(capture.entries).To(ContainElement(ContainSubstring("parent-1 started")))
//
// # Example: SimpleScenario Breakdown
//
// Let's analyze the SimpleScenario configuration step by step:
//
//	children:
//	  - name: "parent-1"              # Unique ID for this worker
//	    workerType: "parent"          # References example-parent worker
//	    userSpec:
//	      config: |
//	        children_count: 2         # ParentUserSpec.ChildrenCount = 2
//
// What happens:
//
// 1. ApplicationSupervisor parses YAMLConfig and finds one child: "parent-1"
//
// 2. It looks up "parent" worker type in the registry (registered by example-parent's init())
//
// 3. It calls ParentWorker.ParseUserSpec() with config: "children_count: 2"
//    This returns ParentUserSpec{ChildrenCount: 2}
//
// 4. It calls ParentWorker.GetChildSpecs() which returns 2 ChildSpec entries:
//    - name: "parent-1-child-1", workerType: "child"
//    - name: "parent-1-child-2", workerType: "child"
//
// 5. For each child, it looks up "child" worker type and repeats the process
//
// 6. The final hierarchy:
//    app-001 (ApplicationSupervisor)
//      └─ parent-1 (ParentWorker)
//          ├─ parent-1-child-1 (ChildWorker)
//          └─ parent-1-child-2 (ChildWorker)
//
// 7. Each worker's FSM independently manages its lifecycle, with state stored in
//    the triangular store (Identity, Desired, Observed collections)
//
// # Advanced Topics
//
// ## Variable Inheritance
//
// Children inherit their parent's variables. The parent can pass down variables
// when creating ChildSpecs:
//
//	childSpec := config.ChildSpec{
//	    Name: "child-1",
//	    UserSpec: config.UserSpec{
//	        Variables: config.VariableBundle{
//	            User: map[string]any{
//	                "parent_ip": parentUserSpec.IP,
//	            },
//	        },
//	    },
//	}
//
// ## Template Expansion
//
// When userSpec.config contains {{ .VAR }}, it's expanded before parsing:
//
//	userSpec:
//	  config: |
//	    url: tcp://{{ .IP }}:{{ .PORT }}
//	  variables:
//	    user:
//	      IP: "192.168.1.100"
//	      PORT: 1883
//
// After expansion:
//
//	url: tcp://192.168.1.100:1883
//
// ## Location Path Computation
//
// If parent has location [enterprise: "ACME", site: "Factory"] and child adds
// [line: "Line-A"], the child's location_path becomes "ACME.Factory.Line-A".
//
// This is available as {{ .location_path }} in templates.
//
// ## Dynamic Child Creation
//
// Parents can dynamically create children based on runtime conditions:
//
//	func (w *ParentWorker) GetChildSpecs(userSpec interface{}) ([]config.ChildSpec, error) {
//	    spec := userSpec.(*ParentUserSpec)
//	    children := make([]config.ChildSpec, spec.ChildrenCount)
//
//	    for i := 0; i < spec.ChildrenCount; i++ {
//	        children[i] = config.ChildSpec{
//	            Name:       fmt.Sprintf("child-%d", i+1),
//	            WorkerType: "child",
//	        }
//	    }
//
//	    return children, nil
//	}
//
// This allows scenarios to create variable numbers of workers based on configuration.

// SimpleScenario demonstrates the minimal FSM v2 hierarchy: one parent with two children.
//
// Hierarchy Created:
//   - 1 parent worker (workerType: "parent")
//   - 2 child workers (dynamically created by parent based on children_count: 2)
//
// This scenario tests:
//   - Worker registration and discovery
//   - Parent-child relationships
//   - Dynamic child creation based on parent configuration
//   - Basic FSM lifecycle (start, run, stop)
//   - State persistence in triangular store
//
// Worker Types Used:
//   - "parent": example-parent worker (creates N children based on config)
//   - "child": example-child worker (no children, just demonstrates lifecycle)
//
// Expected Behavior:
//   1. ApplicationSupervisor creates parent-1
//   2. parent-1 creates parent-1-child-1 and parent-1-child-2
//   3. All workers transition through FSM states (starting → running)
//   4. Workers persist state to triangular store
//   5. On shutdown, all workers transition to stopped
//
// Usage in Tests:
//
//	It("should create parent with 2 children", func() {
//	    done, err := examples.Run(ctx, examples.RunConfig{
//	        Scenario:     examples.SimpleScenario,
//	        Duration:     5 * time.Second,
//	        TickInterval: 100 * time.Millisecond,
//	        Logger:       testLogger,
//	        Store:        store,
//	    })
//	    Expect(err).NotTo(HaveOccurred())
//	    <-done
//
//	    // Assert on store contents, log messages, etc.
//	})
//
// Usage in CLI:
//
//	go run pkg/fsmv2/cmd/runner/main.go \
//	    --scenario=simple \
//	    --duration=10s \
//	    --tick-interval=100ms
var SimpleScenario = Scenario{
	// Name is the CLI identifier. Use lowercase-with-dashes convention.
	Name: "simple",

	// Description appears in CLI output and documentation.
	// Explain what this scenario tests and what behavior to expect.
	Description: "Single parent worker with 2 dynamically-created child workers",

	// YAMLConfig defines the worker hierarchy.
	// This is the same format as production ApplicationWorker configurations.
	//
	// The children array defines root-level workers. Each can have its own children,
	// creating a tree structure.
	//
	// In this scenario:
	// - We create 1 root worker named "parent-1"
	// - It has workerType "parent", which maps to example-parent worker
	// - Its userSpec.config contains "children_count: 2"
	// - The parent worker parses this into ParentUserSpec{ChildrenCount: 2}
	// - The parent's GetChildSpecs() method returns 2 ChildSpec entries
	// - Each child has workerType "child", which maps to example-child worker
	//
	// The result is a 3-worker hierarchy:
	//   app-001
	//     └─ parent-1
	//         ├─ parent-1-child-1
	//         └─ parent-1-child-2
	YAMLConfig: `
children:
  - name: "parent-1"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 2
`,
}
