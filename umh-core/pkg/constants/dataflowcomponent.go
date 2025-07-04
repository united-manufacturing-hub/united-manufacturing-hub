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

package constants

import "time"

const (
	// DataflowComponentExpectedMaxP95ExecutionTimePerInstance means that an instance will not reconcile if not 45ms are left
	// Note: in the intergation test, we defined an alerting threshold of 80% of the max ticker time, which is 100ms
	// So by setting this to 45 ms, we can ensure that an instance will never start if it triggers the alerting threshold
	DataflowComponentExpectedMaxP95ExecutionTimePerInstance = time.Millisecond * 45 // needs to be higher than BenthosExpectedMaxP95ExecutionTimePerInstance
)

const (
	// Used to set the context timeout for updating the observed state of a DataflowComponent instance
	DataflowComponentUpdateObservedStateTimeout = time.Millisecond * 5
)

const (
	// WaitTimeBeforeMarkingStartFailed is the time before marking a DataflowComponent instance as Startfailed if the underlying benthos has not started and stable
	// Benthos takes some time to start usually and we give an enough buffer time of 15 seconds
	// Default value is 15 seconds
	WaitTimeBeforeMarkingStartFailed = time.Second * 15
)

const (
	// Time to wait for a dataflowcomponent to be active in the action
	DataflowComponentWaitForActiveTimeout = time.Second * 30
)
