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

package fsmv2

import (
	"sync"
	"time"
)

// observationIntervals maps a worker type to its collection cadence. It is in
// this core package (not register, which imports supervisor, nor supervisor,
// which would not be visible to the fsmv1 adapter) so both the supervisor's
// collector and the fsmv1 adapter read one source of truth without an import
// cycle.
var observationIntervals sync.Map // map[string]time.Duration

// RegisterObservationInterval records the collection cadence for a worker type,
// typically from simple.Spec.Interval at registration. A non-positive duration
// is ignored so the caller falls back to its own default. Safe for concurrent
// use; last write wins.
func RegisterObservationInterval(workerType string, interval time.Duration) {
	if interval <= 0 {
		return
	}

	observationIntervals.Store(workerType, interval)
}

// ObservationIntervalFor returns the registered collection cadence for a worker
// type and whether one was registered. When ok is false the caller applies its
// own default (the supervisor's DefaultObservationInterval); this package does
// not import that constant to stay cycle-free.
func ObservationIntervalFor(workerType string) (time.Duration, bool) {
	v, ok := observationIntervals.Load(workerType)
	if !ok {
		return 0, false
	}

	return v.(time.Duration), true
}
