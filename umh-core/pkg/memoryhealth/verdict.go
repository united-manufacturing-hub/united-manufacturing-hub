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

package memoryhealth

// State is the health state of a memory verdict.
type State string

const (
	StateHealthy  State = "healthy"
	StateDegraded State = "degraded"
)

// CauseKind names what degraded the memory.
type CauseKind string

const (
	CauseKindContainerLimitFull CauseKind = "container-limit-full"
	CauseKindHostMemoryFull     CauseKind = "host-memory-full"
	CauseKindPressure           CauseKind = "pressure"
	CauseKindOomKill            CauseKind = "oom-kill"
)

// Cause is one reason the memory is degraded, with the windowed value behind it.
type Cause struct {
	Kind  CauseKind `json:"kind"`
	Value float64   `json:"value"`
}

// Verdict is Decide's judgement of the memory, its causes ranked worst first.
type Verdict struct {
	State  State   `json:"state"`
	Causes []Cause `json:"causes,omitempty"`
}
