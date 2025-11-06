package state

// BaseCommunicatorState defines the interface that all communicator state implementations must satisfy.
//
// # Purpose
//
// This interface provides the contract for the State pattern in the Communicator FSM.
// All concrete states (Stopped, TryingToAuthenticate, Syncing, Degraded) embed this
// interface to participate in the state machine lifecycle.
//
// # Design Pattern
//
// This is the State interface from the Gang of Four State pattern:
//   - States implement Next() to define transitions and actions
//   - States implement String() and Reason() for observability
//   - Worker calls Next() on current state during supervision cycles
//
// # Embedding Pattern
//
// Concrete state structs embed BaseCommunicatorState as an anonymous field:
//
//	type StoppedState struct {
//	    BaseCommunicatorState
//	}
//
// This enables type assertions and polymorphism while maintaining a zero-allocation
// interface marker. The interface is intentionally empty because all behavior is
// defined through methods on concrete types.
//
// # State Lifecycle
//
// Each state must implement:
//   - Next(snapshot) - Determines state transitions and actions to execute
//   - String() - Returns human-readable state name for logging
//   - Reason() - Returns user-facing explanation of current state
//
// The FSM v2 Supervisor calls Next() periodically with the current snapshot
// (observed + desired state). The returned values determine the FSM's behavior:
//   - Next state: Which state to transition to (may be self for loops)
//   - Signal: Control signal (SignalNone, SignalNeedsRemoval, etc.)
//   - Action: Idempotent operation to execute (may be nil)
//
// # Related Components
//
// See worker.go for complete FSM architecture and invariants C1-C5.
// See snapshot.go for observed and desired state definitions.
// See action/ package for available actions states can emit.
type BaseCommunicatorState interface {
}
