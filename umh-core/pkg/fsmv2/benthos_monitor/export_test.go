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

package fsmv2benthosmonitor

import "time"

// SetScrapeClock makes workers created after the call stamp their scrapes with
// clock, and returns a function that restores the previous clock. A worker
// created before the call keeps its clock. The clock is package state, so a
// test that sets it must not run in parallel with one that creates workers.
func SetScrapeClock(clock func() time.Time) (restore func()) {
	if clock == nil {
		panic("SetScrapeClock: nil clock; call the returned restore instead")
	}

	prev := scrapeClock
	scrapeClock = clock

	return func() { scrapeClock = prev }
}
