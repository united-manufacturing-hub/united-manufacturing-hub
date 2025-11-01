# RFC FSM Library

<Summary>

## The Problem

The current implementation of FSMs in umh-core using looplab/fsm requires a complex mental model. Go developers are used to having everything explicit, but the FSM layer adds an abstraction. It is therefore not possible to follow the code from top to bottom. Instead one needs to think about multiupkle executions and the states it moves between. this makes it difficult to follow.

Also it has a lot of boilerplate code and the error handling is very chaotic (some in the supervisor, some in the boilerplate part of the FSM, some in the actual business logic)

Furthermore, there are some limitations:
1. The current state is only kept in memory, which resets on a restart. This makes it impossible to do proper High Availability
2. Some features requires the concept of an “Archive Storage” - versioning, audit trails, “history of states”

## The Solution

A simplified interface for creating and managing an FSM integratabtle with the “Control Sync Engine” (see also separate RFC).

There is the concept of a worker, and of a supervisor (similar to what currently exist as FSM and manager).

A worker has an identity (unique ID, etc.), a desired state (that is derived from the user configuration), and an observed state (which is gathered from monitoring).

```
        IDENTITY
       (What it is)
           /\
          /  \
         /    \
        /      \
    DESIRED  OBSERVED
  (Want)      (Have)
       \      /
        \    /
      RECONCILIATION
      (Make have = want)
```


### Responsibilities of the worker

The worker only consists the actual business logic, which the developer needs to satisfy by satisfying the interfaces for `worker` and `state`:

```
type Worker interface {

    CollectObservedState() (observed, error)  // Blocking- potentially long-running collector function that retrieves the observed state (so that it can be stored by the supervisor in the database)

    DeriveDesiredState(spec) (desired, error)    // Pure function that takes in the spec, applies templating/variables, etc and derives the final desired state so that the supervsior can store it into the DB.

    // Provide initial state only
    GetInitialState() State
}

// State interface - where the logic lives
type State interface {
    Next(snapshot Snapshot) (State, Signal, Action)
    String() string
}

```

Each state MUST handle ShutdownRequested explicitly at the beginning of its Next() method. This makes shutdown paths visible and ensures proper cleanup sequencing.

```go
type RunningState struct{}

func (s RunningState) Next(snapshot Snapshot) (State, Signal, Action) {
    // EXPLICIT: First check shutdown
    if snapshot.Desired.ShutdownRequested {
        // I'm running, so I need to stop first
        return StoppingState{}, SignalNone, nil
    }

    // ...
}
```

```go
type StoppedState struct{}

func (s StoppedState) Next(snapshot Snapshot) (State, Signal) {
    // EXPLICIT: When stopped and shutdown requested
    if snapshot.Desired.ShutdownRequested {
        // Now I can start cleanup
        return DeletingState{}, SignalNone, nil
    }

    // ...
}
```

```
type DeletedState struct{}

func (s DeletedState) Next(snapshot Snapshot) (State, Signal) {
    // Final state - signal removal
    return DeletedState{}, SignalNeedsRemoval, nil
}
```



### Responsibilities of the supervisor

The supervisor is then responsible for:
1. Calling the methods of the worker interface `DeriveDesiredState` and `NextState` in sequence (tick-based), and storing the result in a database
2. Ensures that during `Create()`, that will start a new goroutine that will in regular intervals call `CollectObservedState()`.
3. `Tick(snapshot)`, that will do this:
	1. Evaluate the state of the collector goroutine: how old is the latest value in the database from the observed state? Is the data stale (e.g., after 10 seconds) or maybe even “broken” (e.g., if it has been stale for another 10 seconds) and requires a restart? Are there errors appearing in CollectObservedState()?
	2. Calls `DeriveDesiredState(spec)`, where spec is the user configuration derived from the snapshot. Whatever desired state is returned here, is stored into the database\
	3. Calls `worker.CurrentState.Next(snapshot)` where snapshot is the latest available identity, observed state, desired state (see also RFC “CSE”). Upon
4. If an error occurs, it will check if the error is of the type “Retryable” or “Permanent”. Retryable errors are timeouts of all sorts and should be retried in the next tick with ExponentialBackoff and will be at one point of time if they keep repeating become a Permanent error. Permanent errors should cause a removal of the worker.
5. If a worker needs to be removed, the supervisor will set `ShutdownRequested` to true in the desired state. It is required as there is external state of the worker that needs to be cleaned up (e.g., the S6 files). This then causes the worker to reconcile towards a stopped state, and then towards removing itself
6. A worker can also request itself to be restarted, e.g., if it detected config changes that cannot be done online and that require a re-creation.
7. Shutdowns happen over multiple ticks
8. If the supervisor receives the `SignalNeedsRemoval` signal, it means the worker has successfully removed itself and we can remove it from our supervisor
9. If the supervisor receives the `SignalNeedsRestart` signal, it will set the `ShutdownRequested` to true.


Because the supervisor is taking care of the "tick" and not the worker itself, the supervisor can track "ticks in current state" for timeout detection, and also detect workers that are "stuck" (they are not progressing through states)

#### Design Decision: Data Freshness and Collector Health

The FSM separates two orthogonal concerns that could otherwise become entangled:

1. **Infrastructure concern** (supervisor): Is the observation collector working?
2. **Application concern** (states): Is the application/service healthy?

**Collector health is a supervisor concern, not a state concern.** States never see stale data - they assume observations are always fresh and focus purely on business logic (e.g., "is container CPU healthy?").

The supervisor checks data freshness **before** calling `state.Next()`. If observations are stale, the supervisor handles it:
- Pauses the FSM (doesn't call state transitions)
- Attempts to restart the collector with exponential backoff
- Escalates to graceful shutdown if collector cannot be recovered

**Why this separation matters:**

- **Simplicity**: States are pure business logic without infrastructure concerns - easier to write and test
- **Safety**: No risk of making decisions on stale data (e.g., thinking CPU is 50% when it's actually 95% and about to OOM)
- **Reliability**: Automatic collector recovery with clear escalation paths
- **Observability**: Clear separation makes it obvious where failures occur (collector infrastructure vs application health)

#### State Naming Convention

FSM v2 uses a strict naming convention to make state behavior immediately obvious:

**PASSIVE states** (observe and transition based on conditions):
- Use descriptive nouns/adjectives: `ActiveState`, `DegradedState`, `StoppedState`
- **Never emit actions** - only evaluate conditions and return next state
- Examples: Health monitoring, status observation

**ACTIVE states** (perform work with retry logic):
- Use "TryingTo" prefix: `TryingToStartState`, `TryingToAuthenticateState`, `TryingToStopState`
- **Always emit actions on every tick** until success
- Examples: Authentication, initialization, cleanup operations

**When to skip transition states:**
- If transitioning requires NO work (no actions), skip the intermediate state
- ❌ Bad: `Stopped` → `StartingState` (passive, no action) → `Active`
- ✅ Good: `Stopped` → `Active` (direct transition)
- ✅ Good: `Stopped` → `TryingToStartState` (emits `InitializeAction`) → `Active`

**Examples:**
```go
// PASSIVE: Monitoring FSMs (Container, Agent)
// No startup/shutdown work needed
Stopped → Active → Degraded ↔ Active → Stopped

// ACTIVE: Component FSMs (Communicator, Benthos)
// Require actions with retry logic
Stopped → TryingToAuthenticateState → TryingToConnectState → Active → TryingToStopState → Stopped
```

**Rule of thumb:** If the state name uses a gerund ("-ing") but doesn't emit actions, it's named wrong. Either add the action or rename to a descriptive noun.
