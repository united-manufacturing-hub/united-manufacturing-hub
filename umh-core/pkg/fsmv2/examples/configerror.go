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

// ConfigErrorScenario demonstrates configuration validation and error handling.
//
// # What This Scenario Tests
//
// This scenario showcases how FSM v2 handles various configuration errors:
//
//  1. Invalid Worker Type
//     - Worker type that doesn't exist in registry
//     - Demonstrates graceful handling of registration errors
//
//  2. Type Mismatch in Configuration
//     - String value where integer expected
//     - YAML parsing produces zero values or errors
//
//  3. Valid Configuration Baseline
//     - Correctly configured worker for comparison
//     - Shows what "working" looks like alongside errors
//
// # Configuration Error Flow
//
// When configuration is invalid, the flow depends on where validation fails:
//
// ## Worker Type Not Found
//
//  1. ApplicationSupervisor parses YAML
//  2. For each child, looks up workerType in factory registry
//  3. If not found: error logged, worker not created
//  4. Other workers continue normally
//
// ## YAML Parse Error
//
//  1. DeriveDesiredState calls ParseUserSpec[T](spec)
//  2. ParseUserSpec attempts yaml.Unmarshal
//  3. If type mismatch: Go uses zero value (no error from unmarshal!)
//  4. Worker proceeds with default/zero config
//
// ## Missing Required Field
//
//  1. Worker receives parsed config with zero values
//  2. Worker code must validate and handle missing fields
//  3. GetMaxFailures() returns default (3) if MaxFailures is 0
//
// # Key Insight: YAML Parsing is Lenient
//
// Go's yaml.Unmarshal is very forgiving:
//
//	config: |
//	  children_count: "not-a-number"
//
// This does NOT produce an error! The children_count field remains 0.
// This is why worker code often has defaults:
//
//	func (s *Spec) GetMaxFailures() int {
//	    if s.MaxFailures <= 0 {
//	        return 3 // Default
//	    }
//	    return s.MaxFailures
//	}
//
// # What to Observe
//
// Invalid worker type:
//
//	"worker type not found" workerType="nonexistent_worker"
//	"skipping child creation" name="config-invalid-type"
//
// Type mismatch (children_count: "not-a-number"):
//
//	Worker starts normally with children_count = 0 (zero value)
//	No explicit error message - silent degradation
//	Parent creates no children
//
// Valid configuration:
//
//	Normal startup sequence
//	Parent creates expected number of children
//
// # Recommendations for Production
//
// ## Explicit Validation
//
//	func (s *MySpec) Validate() error {
//	    if s.Timeout <= 0 {
//	        return fmt.Errorf("timeout must be positive, got %d", s.Timeout)
//	    }
//	    if s.MaxRetries < 0 {
//	        return fmt.Errorf("max_retries cannot be negative")
//	    }
//	    return nil
//	}
//
// ## Fail-Fast in DeriveDesiredState
//
//	func (w *Worker) DeriveDesiredState(spec interface{}) (DesiredState, error) {
//	    parsed, err := config.ParseUserSpec[MySpec](spec)
//	    if err != nil {
//	        return DesiredState{}, fmt.Errorf("invalid config: %w", err)
//	    }
//	    if err := parsed.Validate(); err != nil {
//	        return DesiredState{}, fmt.Errorf("config validation failed: %w", err)
//	    }
//	    return DesiredState{...}, nil
//	}
//
// ## Semantic Validation in Actions
//
//	func (a *ConnectAction) Execute(ctx context.Context, deps any) error {
//	    d := deps.(*Dependencies)
//	    if d.GetURL() == "" {
//	        return fmt.Errorf("cannot connect: URL not configured")
//	    }
//	    // ... actual connection logic
//	}
//
// # Configuration Error Categories
//
//	Category              | When Detected      | Effect
//	---------------------|-------------------|-------------------------
//	Invalid worker type  | Supervisor startup | Worker not created
//	YAML syntax error    | ParseUserSpec      | DeriveDesiredState error
//	Type mismatch        | YAML unmarshal     | Zero value (silent!)
//	Missing required     | Worker validation  | Validation error
//	Invalid semantics    | Action execution   | Action failure
//
// # Real-World Parallels
//
// Invalid Worker Type:
//   - Typo in configuration file
//   - Missing plugin/module
//   - Version mismatch (old config, new code)
//
// Type Mismatch:
//   - "timeout: fast" instead of "timeout: 30"
//   - Copy-paste errors from examples
//   - Template variable not substituted
//
// Missing Required:
//   - New required field added in update
//   - Partial configuration during development
//   - Environment variable not set
var ConfigErrorScenario = Scenario{
	Name: "configerror",

	Description: "Demonstrates configuration validation and error handling patterns",

	YAMLConfig: `
children:
  # Valid configuration baseline
  - name: "config-valid"
    workerType: "exampleparent"
    userSpec:
      config: |
        children_count: 2

  # Type mismatch: children_count becomes 0 (YAML is lenient)
  - name: "config-type-mismatch"
    workerType: "exampleparent"
    userSpec:
      config: |
        children_count: "not-a-number"

  # Empty config: all fields get zero values
  - name: "config-empty"
    workerType: "exampleparent"
    userSpec:
      config: ""

  # Invalid max_failures becomes 0, GetMaxFailures() returns default 3
  - name: "config-failing-defaults"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: "three"
`,
}
