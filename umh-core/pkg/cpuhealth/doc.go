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

// Package cpuhealth decides whether the umh-core container is CPU starved.
//
// High CPU usage alone is not degradation: a container that uses its full
// quota while making progress is busy, not sick. The verdict therefore keys
// on starvation evidence rather than utilization: CFS throttling, PSI
// pressure, hypervisor steal time, and the host running out of spare cores.
//
// # The two-rule ceiling
//
// One question selects the rule: does the container have a CPU limit (a
// cgroup quota)? [Decide] evaluates the limit rule first, so a limit-set
// sample never reaches the no-limit arms.
//
//   - No limit: the ceiling is the host. Headroom is LogicalCpus minus the
//     60s mean of host-busy cores minus a one-core reserve
//     (cpuReserveCores); the saturation latch fires when headroom drops
//     below 0, that is, when the host as a whole has less than about one
//     core free.
//   - Limit set: the ceiling is the quota. Headroom is the quota minus the
//     container's own 60s-avg core usage minus a fractional reserve
//     (Thresholds.LimitReserveFraction of the quota); the latch fires when
//     sustained usage enters the reserve band. Sustained throttling
//     degrades independently of headroom.
//
// # Causes and sub-latches
//
// The emitted saturation state is the OR of four internal latches, each
// pinning a specific cause so the operator message and the wire basis can
// name it:
//
//   - limitSaturationFired: limit mode, the container's sustained usage is
//     inside the quota's reserve band.
//   - hostFullFired: limit mode, the host itself is full (host headroom
//     below 0). A limit is a ceiling, not a reservation, so a full host
//     stacks as a cause on a limited container. Evaluated only when
//     /proc/stat is readable; holds across an outage.
//   - noLimitHostFired: no limit, host stats readable, host headroom
//     below 0.
//   - noHostStatsSaturationFired: no limit and /proc/stat unreadable. The
//     fallback fires on sustained container usage at or above 70 percent
//     (Thresholds.HighUsageFraction) of the machine's logical cores.
//
// Throttle, pressure, and steal are independent latches beside saturation:
// throttle from the 60s cpu.stat counter-delta ratio, pressure from the
// kernel's cpu.pressure "some avg60" thresholded directly, and steal from
// the 60s p95 of /proc/stat steal (virtualized boxes only). When several
// causes fire, they sort dominant-first (throttle, pressure, and steal rank
// above saturation) and the dominant cause drives the verdict's
// Attribution: steal and a full host attribute to the host; otherwise the
// 60s host/container usage split decides between host and unknown.
//
// # Hysteresis and missing data
//
// Every latch is a Schmitt trigger: it fires at a high threshold and
// recovers at a lower one ([DefaultThresholds] lists both marks), so the
// verdict does not flap when a signal sits at the boundary. The same
// discipline extends to missing data: a failed read is not evidence of
// health. Ring appends are gated on the readability flags carried by
// [Sample] (NrPeriodsAvailable for the throttle ring,
// HostBusyCoresAvailable for the host-busy ring, PsiReadable for the
// pressure latch), the affected latch holds its prior state across the
// outage, and the container monitor holds the last verdict across a
// sampler failure instead of running [Decide] on a zero sample.
//
// A new signal must follow the same discipline: gate its append on a
// readability flag and hold its latch when the read fails; never treat a
// zero-on-failure as a measurement.
//
// # Division of labor and the wire
//
// sampler.go reads the inputs once per tick through the injected
// filesystem service: cpu.stat (usage and throttle counters), cpu.max, and
// cpu.pressure under the injected base path, plus /proc/stat (host busy and
// steal) and the virtualization probes. decide.go is a state machine over
// those samples: [Decide] is the only mutator of [WindowState]. message.go
// composes the operator-facing text ([ComposeMessage], [BlockReason]).
//
// The container monitor (pkg/service/container_monitor) owns the tick: it
// calls the [Sampler], gates [Decide] on sampler success, and emits the
// structured verdict basis on the wire for the Management Console. A
// degraded verdict blocks new bridge deployment through the protocol
// converter's resource limiting.
//
// See docs/production/cpu-health.md for the operator-facing description of
// the statuses and remediation.
package cpuhealth
