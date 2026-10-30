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

// Unit is the unit a cause's value is denominated in.
type Unit string

// Attribution names where a cause sits: inside this container or on the machine.
type Attribution string

const (
	AttributionUnknown   Attribution = "unknown"
	AttributionHost      Attribution = "host"
	AttributionContainer Attribution = "container"
)

const (
	blameHost = iota + 1
	blameContainer
)

// Cause is one reason the memory is degraded, with the windowed value behind it.
type Cause struct {
	Kind        CauseKind   `json:"kind"`
	Instrument  string      `json:"instrument"`
	Unit        Unit        `json:"unit"`
	Attribution Attribution `json:"attribution"`
	Value       float64     `json:"value"`
}

// Verdict is Decide's judgement of the memory, its causes ranked worst first.
// Attribution is the first cause's.
type Verdict struct {
	State       State       `json:"state"`
	Attribution Attribution `json:"attribution,omitempty"`
	Causes      []Cause     `json:"causes,omitempty"`
}

func nameAttribution(blame int) Attribution {
	switch blame {
	case blameHost:
		return AttributionHost
	case blameContainer:
		return AttributionContainer
	default:
		return AttributionUnknown
	}
}
