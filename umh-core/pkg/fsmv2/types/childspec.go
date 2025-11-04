// Package types provides common data structures for FSM v2 workers.
// These types enable declarative child management and structured state representation.
package types

import "encoding/json"

// UserSpec contains user-provided configuration for a worker.
// This is the "raw" configuration that users write, before templating or transformation.
//
// The supervisor passes this to Worker.DeriveDesiredState() where it's transformed
// into technical configuration (DesiredState). This separation allows workers to:
//   - Parse and validate user input
//   - Apply templates and variable substitution
//   - Add computed/derived settings
//   - Normalize configuration formats
//
// Example flow:
//
//	UserSpec{Config: "host: {{ .IP }}\nport: {{ .PORT }}"}
//	        ↓ DeriveDesiredState()
//	DesiredState{Host: "192.168.1.100", Port: 502}
type UserSpec struct {
	Config string `yaml:"config" json:"config"` // Raw user-provided configuration (YAML, JSON, or other format)
}

// ChildSpec is a declarative specification for a child FSM worker.
// Parent workers return these in DeriveDesiredState().ChildrenSpecs to declare their children.
// The supervisor reconciles actual children to match these specs (Kubernetes-style).
//
// DECLARATIVE CHILD MANAGEMENT:
// Parents don't create/destroy children directly. Instead they declare what should exist,
// and the supervisor handles creation, updates, and cleanup automatically:
//
//   1. Parent returns []ChildSpec in DesiredState.ChildrenSpecs
//   2. Supervisor compares with actual children
//   3. Supervisor creates missing children
//   4. Supervisor updates changed children
//   5. Supervisor removes extra children
//
// This enables clean separation of concerns:
//   - Parents focus on "what should exist"
//   - Supervisor handles "how to make it exist"
//   - Children run independently in their own FSMs
//
// STATE MAPPING (optional):
// Parents can map their own state to child states. This allows parents to say
// "when I'm in state X, my children should be in state Y":
//
//	StateMapping: map[string]string{
//	    "running":  "active",   // When parent is running, children should be active
//	    "stopping": "stopped",  // When parent is stopping, children should stop
//	}
//
// Example - Protocol converter managing connections:
//
//	// Parent (protocol converter) declares a child (MQTT connection)
//	ChildSpec{
//	    Name:       "mqtt-connection",
//	    WorkerType: "mqtt_client",
//	    UserSpec:   UserSpec{Config: "url: tcp://localhost:1883"},
//	    StateMapping: map[string]string{
//	        "idle":    "stopped",    // When converter idle, disconnect
//	        "active":  "connected",  // When converter active, connect
//	        "closing": "stopped",    // When converter closing, disconnect
//	    },
//	}
//
// Example - Benthos managing connections and data flows:
//
//	// Benthos declares multiple children with different mappings
//	[]ChildSpec{
//	    {
//	        Name:       "modbus-connection",
//	        WorkerType: "modbus_client",
//	        UserSpec:   UserSpec{Config: "address: 192.168.1.100:502"},
//	    },
//	    {
//	        Name:       "source-flow",
//	        WorkerType: "benthos_flow",
//	        UserSpec:   UserSpec{Config: "input: {...}"},
//	        StateMapping: map[string]string{
//	            "running": "active",
//	            "stopped": "stopped",
//	        },
//	    },
//	}
type ChildSpec struct {
	Name         string            `yaml:"name" json:"name"`                                   // Unique name for this child (within parent scope)
	WorkerType   string            `yaml:"workerType" json:"workerType"`                       // Type of worker to create (registered worker factory key)
	UserSpec     UserSpec          `yaml:"userSpec" json:"userSpec"`                           // Configuration for the child worker
	StateMapping map[string]string `yaml:"stateMapping,omitempty" json:"stateMapping,omitempty"` // Optional parent→child state mapping
}

// MarshalJSON implements json.Marshaler for ChildSpec.
// This ensures consistent JSON serialization across the system.
func (c *ChildSpec) MarshalJSON() ([]byte, error) {
	type Alias ChildSpec
	return json.Marshal((*Alias)(c))
}

// DesiredState represents what we want the system to be.
// This is returned by Worker.DeriveDesiredState() and used by State.Next() for decisions.
//
// The supervisor can inject shutdown requests by setting State to "shutdown".
// Workers MUST check ShutdownRequested() first in their State.Next() implementations.
//
// CHILDREN MANAGEMENT:
// The ChildrenSpecs field enables declarative child management. Parent workers populate
// this to declare what children should exist. The supervisor handles all lifecycle:
//
//	// In parent's DeriveDesiredState():
//	func (w *ParentWorker) DeriveDesiredState(spec interface{}) (DesiredState, error) {
//	    return types.DesiredState{
//	        State: "running",
//	        ChildrenSpecs: []types.ChildSpec{
//	            {Name: "child-1", WorkerType: "mqtt_client", ...},
//	            {Name: "child-2", WorkerType: "modbus_client", ...},
//	        },
//	    }, nil
//	}
//
// Example with shutdown:
//
//	DesiredState{
//	    State:         "shutdown",  // Triggers shutdown sequence
//	    ChildrenSpecs: nil,         // Children removed during shutdown
//	}
type DesiredState struct {
	State         string      `yaml:"state" json:"state"`                                       // Current desired state ("running", "stopped", "shutdown", etc.)
	ChildrenSpecs []ChildSpec `yaml:"childrenSpecs,omitempty" json:"childrenSpecs,omitempty"` // Declarative specification of child workers
}

// ShutdownRequested returns true if graceful shutdown has been requested.
// States MUST check this first in their Next() method before any other logic.
//
// This implements the fsmv2.DesiredState interface requirement.
//
// Shutdown flow:
//   1. Supervisor sets State = "shutdown" in DesiredState
//   2. State.Next() calls ShutdownRequested() → returns true
//   3. State transitions to shutdown/cleanup states
//   4. Eventually returns SignalNeedsRemoval
//   5. Supervisor removes worker from system
//
// Example usage in State.Next():
//
//	func (s RunningState) Next(snapshot fsmv2.Snapshot) (State, Signal, Action) {
//	    desired := snapshot.Desired.(types.DesiredState)
//	    // ALWAYS check shutdown first
//	    if desired.ShutdownRequested() {
//	        return StoppingState{}, fsmv2.SignalNone, nil
//	    }
//	    // ... rest of logic
//	}
func (d DesiredState) ShutdownRequested() bool {
	return d.State == "shutdown"
}
