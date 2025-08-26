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

package fsm

const (
	// EventRemove is triggered to remove an instance.
	LifecycleEventRemove = "remove"
	// EventRemoveDone is triggered when the instance has been removed.
	LifecycleEventRemoveDone = "remove_done"
	// EventCreate is triggered to create an instance.
	LifecycleEventCreate = "create"
	// EventCreateDone is triggered when the instance has been created.
	LifecycleEventCreateDone = "create_done"
)

// LifecycleState constants represent the various lifecycle states a Benthos instance can be in
// They will be handled before the operational states.
const (
	// LifecycleStateToBeCreated indicates the instance has not been created yet.
	LifecycleStateToBeCreated = "to_be_created"
	// LifecycleStateCreating indicates the instance is being created.
	LifecycleStateCreating = "creating"
	// LifecycleStateRemoving indicates the instance is being removed.
	LifecycleStateRemoving = "removing"
	// LifecycleStateRemoved indicates the instance has been removed and can be cleaned up.
	LifecycleStateRemoved = "removed"
)

// State type checks.
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
