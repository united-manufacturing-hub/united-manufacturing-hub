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
	// defaultTickerTime is the interval between reconciliation cycles.
	// This value balances responsiveness with resource utilization:
	// - Too small: could mean that the managers do not have enough time to complete their work
	// - Too high: Delayed response to configuration changes
	DefaultTickerTime = 100 * time.Millisecond

	// starvationThreshold defines when to consider the control loop starved.
	// If no reconciliation has happened for this duration, the starvation
	// detector will log warnings and record metrics.
	// Starvation will take place for example when adding hundreds of new services
	// at once.
	StarvationThreshold = 15 * time.Second

	// DefaultManagerName is the default name for a manager.
	DefaultManagerName = "Core"

	// DefaultInstanceName is the default name for an instance.
	DefaultInstanceName = "Core"

	// DefaultMinimumRemainingTimePerManager is the default minimum remaining time for a manager.
	DefaultMinimumRemainingTimePerManager = time.Millisecond * 50

	// maximum times in a row the same manager may return (reconciled = true)
	// before we put it into a cooling‑off period
	StarvationLimit = 3

	// number of control‑loop ticks a manager stays in cooldown
	CoolDownTicks = 5

	// LoopControlLoopTimeFactor allocates time budget for parallel manager execution within control loop.
	//
	// WHY: Reserves 20% of control loop time for error aggregation, cleanup operations, and system
	// snapshot creation after parallel manager execution completes. Prevents timeout failures
	// in the main control loop when individual managers consume their full allocated time.
	//
	// BUSINESS LOGIC: If set too low, parallel execution may not have sufficient time for all
	// managers to complete, reducing system throughput. If set too high, insufficient time
	// remains for error handling and snapshot creation, risking control loop timeout failures.
	//
	// PARALLEL EXECUTION CONTEXT: With parallel execution, multiple managers run concurrently
	// but still need coordination time for:
	//   - Error collection and aggregation from errgroup
	//   - Mutex-protected state updates and logging
	//   - System snapshot generation and persistence
	//   - Cleanup and context cancellation propagation
	LoopControlLoopTimeFactor = 0.80

	RingBufferCapacity = 3
)

// FilesAndDirectoriesToIgnore is a list of files and directories that we will not read.
// All older archived logs begin with @40000000
// As we retain up to 20 logs, this will otherwise lead to reading a lot of logs
var FilesAndDirectoriesToIgnore = []string{".s6-svscan", "s6-linux-init-shutdown", "s6rc-fdholder", "s6rc-oneshot-runner", "syslogd", "syslogd-log", "/control", "/lock", "@40000000"}
