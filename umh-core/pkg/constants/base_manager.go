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

const (
	// BaseManagerControlLoopTimeFactor reserves safety time for FSM manager cleanup operations.
	//
	// WHY: Prevents timeout failures during FSM manager reconciliation by ensuring adequate
	// time remains for cleanup operations, error handling, and state persistence after
	// instance reconciliation completes.
	//
	// BUSINESS LOGIC: If set too low, FSM managers may timeout during cleanup, potentially
	// leaving instances in inconsistent states. If set too high, reduces available time
	// for actual reconciliation work, impacting system responsiveness.
	//
	// CALCULATION: 1.0 - ManagerReservePercent (5%) = 0.95 (95% of manager time)
	// With 90ms manager time: 90ms * 0.95 = 85.5ms for instances, 4.5ms for cleanup.
	BaseManagerControlLoopTimeFactor = 1.0 - ManagerReservePercent
)
