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

// The vocabulary a verdict is written in: its states, its attributions, and
// the kinds of cause it can name.

package cpuhealth

// State is the customer-visible health state of the CPU verdict. Two values and
// no third: limited visibility is an annotation on a healthy verdict, never a
// state.
type State string

const (
	StateHealthy  State = "healthy"
	StateDegraded State = "degraded"
)

// Attribution names the dominant cause class when degraded. Three members:
// host for contention outside this container, container for a cause inside it,
// and unknown when the evidence does not place the cause on either side.
// ENG-5264 widens this to host, workload, software and unknown.
type Attribution string

const (
	AttributionUnknown   Attribution = "unknown"
	AttributionHost      Attribution = "host"
	AttributionContainer Attribution = "container"
)

// The blame values the table declares beside its signals and refinements.
// diagnosis.Signal.Attribution is an int the engine carries and never reads, so
// the numbering is this package's own; nameAttribution names one of them.
// blameUnknown is zero, so a row that declares nothing blames nobody.
const (
	blameUnknown int = iota
	blameHost
	blameContainer
)

// CauseKind enumerates the reason classes that can degrade CPU health.
type CauseKind string

const (
	// CauseKindHostCpuFull's host-headroom arm and CauseKindContainerLimitFull
	// are the same measurement — capacity less the load on it less a reserve,
	// in cores — against two ceilings. The host's cores are one ceiling and the
	// container's own CPU limit is the other, and the remedy differs by which
	// one was reached. The load subtracted differs with the ceiling: the
	// machine's whole busy time against the host's cores, this container's own
	// usage against its own limit.
	CauseKindHostCpuFull        CauseKind = "host-cpu-full"
	CauseKindContainerLimitFull CauseKind = "container-limit-full"
	CauseKindThrottling         CauseKind = "throttling"
	CauseKindPressure           CauseKind = "pressure"
	CauseKindSteal              CauseKind = "steal"
)

// Unit is the unit a cause's value is denominated in, copied from the mark pair
// that judged it. No sentence interpolates it: the message layer picks its copy
// from the cause's kind and instrument, and a Cause carries the unit so a
// reader of one does not have to go back to the table for it.
type Unit string

// cpuReserveCores is the no-limit headroom reserve: one core set aside for
// Redpanda. It is Redpanda's default maxCores (--smp), not a calibration
// guess. Decide stamps it onto Details.ReserveCores so the message reads the
// same number the verdict subtracted.
const cpuReserveCores = 1.0

// limitReserveFraction is the limit-mode headroom reserve: the fraction of a
// container's own CPU quota held back as headroom, cpuReserveCores' pair for
// limit mode. Decide stamps the product onto Details.ReserveCores the same way.
const limitReserveFraction = 0.10

// Cause is one entry in a degraded verdict, ordered by diagnosis.Rank. It says
// what kind of trouble it is, how much, in what unit, and what measured it, so
// the message layer can choose a sentence from the cause alone.
type Cause struct {
	Kind CauseKind
	// Instrument names the instrument that produced this cause, copied from
	// the latch that fired. A signal measured two ways needs it: host-cpu-full
	// read from /proc/stat and host-cpu-full estimated from our own usage earn
	// different sentences.
	Instrument string
	Value      float64
	Unit       Unit
}

// Verdict is what Decide returns: the state, the attribution of the dominant
// cause, and the ranked cause list. The message is NOT a field on it — the
// message layer composes it from Verdict and Details.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}
