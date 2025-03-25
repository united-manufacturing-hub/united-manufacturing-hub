package fsm

const (
	// EventRemove is triggered to remove an instance
	LifecycleEventRemove = "remove"
	// EventRemoveDone is triggered when the instance has been removed
	LifecycleEventRemoveDone = "remove_done"
	// EventCreate is triggered to create an instance
	LifecycleEventCreate = "create"
	// EventCreateDone is triggered when the instance has been created
	LifecycleEventCreateDone = "create_done"
)

// LifecycleState constants represent the various lifecycle states a Benthos instance can be in
// They will be handled before the operational states
const (
	// LifecycleStateToBeCreated indicates the instance has not been created yet
	LifecycleStateToBeCreated = "to_be_created"
	// LifecycleStateCreating indicates the instance is being created
	LifecycleStateCreating = "creating"
	// LifecycleStateRemoving indicates the instance is being removed
	LifecycleStateRemoving = "removing"
	// LifecycleStateRemoved indicates the instance has been removed and can be cleaned up
	LifecycleStateRemoved = "removed"
)

// State type checks
func IsLifecycleState(state string) bool {
	switch state {
	case LifecycleStateToBeCreated,
		LifecycleStateCreating,
		LifecycleStateRemoving,
		LifecycleStateRemoved:
		return true
	default:
		return false
	}
}
