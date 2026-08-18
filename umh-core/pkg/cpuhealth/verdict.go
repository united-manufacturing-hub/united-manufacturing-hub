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
// no third: the dead zone is an annotation on a healthy verdict, never a state.
type State string

const (
	StateHealthy  State = "healthy"
	StateDegraded State = "degraded"
)

// Attribution names the dominant cause class when degraded. Two members
// today: unknown is the complement of host. ENG-5264 widens this to
// host, workload, software and unknown; until it lands, a cause inside
// this container reads unknown rather than naming what caused it.
type Attribution string

const (
	AttributionUnknown Attribution = "unknown"
	AttributionHost    Attribution = "host"
)

// CauseKind enumerates the reason classes that can degrade CPU health.
type CauseKind string

const (
	CauseKindSaturation CauseKind = "saturation"
	CauseKindThrottling CauseKind = "throttling"
	CauseKindPressure   CauseKind = "pressure"
	CauseKindSteal      CauseKind = "steal"
	// CauseKindHostContention is declared but no signal produces it.
	CauseKindHostContention CauseKind = "host-contention"
)

// Unit is the unit a cause's value is denominated in, copied from the mark pair
// that judged it so the message layer can render "cores" vs "ratio".
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

// Cause is one entry in a degraded verdict, ordered by diagnosis.Rank.
type Cause struct {
	Kind  CauseKind
	Value float64
	Unit  Unit
}

// Verdict is what Decide returns: the state, the attribution of the dominant
// cause, and the ranked cause list. The message is NOT a field on it — the
// message layer composes it from Verdict and Details.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}

// saturationArm identifies which instrument of the saturation family a chosen
// cause came from. The constants below are declared in that precedence
// order — no-host-stats, then limit, then host-full — so the arm value IS the
// rank: a later constant outranks every constant declared before it. Do not
// reorder them: doing so silently changes which arm chooseSaturationCause picks.
type saturationArm int

const (
	// noSaturationArm is the unset value a non-saturation fired signal maps to.
	noSaturationArm saturationArm = iota

	// noHostStatsArm is the usage-fraction fallback: this instance's own usage
	// stands in for the host reading when /proc/stat is unreadable.
	noHostStatsArm

	// limitArm is this instance's own CPU quota running out, independent of
	// the host.
	limitArm

	// hostFullArm is the host itself reporting full, from host-headroom's own
	// /proc/stat reading.
	hostFullArm
)
