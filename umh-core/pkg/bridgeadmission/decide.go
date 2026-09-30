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

// Package bridgeadmission decides whether umh-core may create a bridge.
//
// A bridge connects one device to umh-core; the code calls it a protocol
// converter. Bridge admission is the check umh-core runs before it creates a
// bridge. A refused bridge is not created. It waits in the to_be_created state,
// and the check runs again on every later pass of umh-core's control loop, so
// the bridge starts on its own once it is admitted.
//
// # What Decide receives
//
// Decide reads only its Input. It logs nothing, reads no clock and makes no
// system call. The caller gathers the Input and acts on the Decision; today
// that caller is IsResourceLimited in pkg/service/protocolconverter. The Input
// fields are:
//
//   - BlockingEnabled: the config setting agent.enableResourceLimitBlocking,
//     which turns bridge admission on or off.
//   - CPU, Memory and Disk: the health of each resource. Each is Healthy,
//     Degraded or Unknown, with a message explaining it. Unknown means nobody
//     has shown the resource to be healthy yet, for example because the
//     instance has only just started.
//   - Admitted: how many other bridges have already been admitted and are not
//     being removed.
//   - WaitingAhead: how many bridges are still waiting for admission and come
//     before this bridge in config.yaml.
//   - CapacityCores: the container's CPU limit in cores, when it is known.
//   - HostCores: the number of CPU cores on the host.
//
// # The rules, in the order Decide applies them
//
// The first rule that matches decides.
//
//  1. Bridge admission is off: admit the bridge and check nothing else. This
//     setting exists to start bridges in an emergency while a resource problem
//     is unfixed.
//  2. A resource is Degraded: refuse, and name the resource and its message,
//     so the user knows what to fix. CPU is checked first, then memory, then
//     disk. This rule comes before rule 3, so a known problem is named even
//     while another resource has no reading.
//  3. A resource is Unknown: refuse with "Resource health not proven yet". A
//     bridge is admitted only on health that a reading has shown, so a fresh
//     instance starts no bridges until its first health readings arrive.
//  4. No place under the bridge limit is free: refuse. The next section
//     defines the limit and its places.
//  5. Otherwise: admit the bridge.
//
// Every refusal carries a hint: Decision.Message appends how to turn bridge
// admission off with agent.enableResourceLimitBlocking: false.
//
// # The bridge limit
//
// The limit is (cores - 1) * 5 bridges, rounded down and never below zero.
// cores is CapacityCores when it is known and above zero, and HostCores
// otherwise. One core is reserved for Redpanda, the message broker that runs
// inside umh-core. Each remaining core may run BridgesPerCore bridges.
// docs/production/sizing-guide.md recommends these figures.
//
// A place is one bridge's share of the limit. Each admitted bridge takes a
// place. Each waiting bridge ahead of this one in config.yaml takes a place
// too, because it is admitted before this one. Waiting bridges behind it take
// no place. The bridge is admitted when at least one place is left free.
//
// After a restart no bridge is admitted yet and every bridge waits. The first
// bridges in config.yaml order are then admitted up to the limit. The rest
// wait until a place frees up, for example when a bridge is removed.
package bridgeadmission

import "fmt"

// BridgesPerCore is how many bridges each CPU core may run once
// redpandaReservedCores are set aside. docs/production/sizing-guide.md
// recommends this figure.
const BridgesPerCore = 5

// redpandaReservedCores is the CPU, in cores, that the bridge limit leaves to
// Redpanda.
const redpandaReservedCores = 1

// emergencyHint says how to start bridges anyway while a resource problem is unfixed.
const emergencyHint = "In an emergency, you can start bridges anyway by setting agent.enableResourceLimitBlocking: false in the instance's Config File. It takes effect without a restart. Set it back to true once the resource problem is fixed."

// Health is what the caller knows about one resource.
type Health int

const (
	// Unknown means nobody has shown the resource to be healthy: no monitor,
	// no observation yet, or the instance is not active yet.
	Unknown Health = iota
	// Healthy means an observation showed the resource to be healthy.
	Healthy
	// Degraded means an observation showed the resource to be degraded.
	Degraded
)

// Resource is the health of one resource and the message explaining it.
// Message may be empty. For Unknown it says why the health is not known.
type Resource struct {
	Health  Health
	Message string
}

// Input is everything Decide reads.
type Input struct {
	// BlockingEnabled is the config setting agent.enableResourceLimitBlocking.
	BlockingEnabled bool

	CPU    Resource
	Memory Resource
	Disk   Resource

	// Admitted counts the bridges that already passed admission and are not
	// being removed. It excludes the bridge being decided.
	Admitted int
	// WaitingAhead counts the bridges that are still waiting for admission and
	// come before the bridge being decided in config.yaml.
	WaitingAhead int

	// CapacityCores is the container's CPU limit in cores, or nil when it is
	// not known. Decide treats a value of zero or less as not known.
	CapacityCores *float64
	// HostCores is the number of cores on the host. Decide uses it when
	// CapacityCores is not known.
	HostCores int
}

// Cause names why a bridge was refused.
type Cause int

const (
	// None means the bridge is admitted.
	None Cause = iota
	// NotProven means at least one resource is Unknown.
	NotProven
	// CPU means the CPU is Degraded.
	CPU
	// Memory means memory is Degraded.
	Memory
	// Disk means the disk is Degraded.
	Disk
	// BridgeLimit means admitting the bridge would exceed the bridge limit.
	BridgeLimit
)

// Decision is the result of Decide.
type Decision struct {
	Admit  bool
	Cause  Cause
	Reason string
	// Limit is the maximum number of bridges for this instance, or nil when
	// Decide refused before computing it.
	Limit *int
}

// Message returns the text shown for a refused bridge: Reason followed by how
// to start bridges anyway. It returns "" for an admitted bridge.
func (d Decision) Message() string {
	if d.Admit {
		return ""
	}

	return d.Reason + ". " + emergencyHint
}

// degradedDecision refuses because one resource is degraded. The reason is
// prefix followed by the resource's message, or generic when the message is empty.
func degradedDecision(cause Cause, r Resource, prefix, generic string) Decision {
	reason := generic
	if r.Message != "" {
		reason = prefix + r.Message
	}

	return Decision{Cause: cause, Reason: reason}
}

// notProvenDecision refuses because a resource is Unknown. The reason carries
// the Unknown resource's message when it has one.
func notProvenDecision(r Resource) Decision {
	reason := "Resource health not proven yet"
	if r.Message != "" {
		reason += ": " + r.Message
	}

	return Decision{Cause: NotProven, Reason: reason}
}

// Decide returns whether a bridge may be created. The package doc lists the
// rules in the order Decide applies them.
func Decide(in Input) Decision {
	if !in.BlockingEnabled {
		return Decision{Admit: true, Cause: None}
	}

	switch {
	case in.CPU.Health == Degraded:
		return degradedDecision(CPU, in.CPU, "CPU degraded: ", "CPU resources degraded")
	case in.Memory.Health == Degraded:
		return degradedDecision(Memory, in.Memory, "Memory degraded: ", "Memory resources degraded")
	case in.Disk.Health == Degraded:
		return degradedDecision(Disk, in.Disk, "Disk degraded: ", "Disk resources degraded")
	}

	for _, r := range []Resource{in.CPU, in.Memory, in.Disk} {
		if r.Health == Unknown {
			return notProvenDecision(r)
		}
	}

	cores := limitCores(in)
	coresForBridges := max(cores-redpandaReservedCores, 0)
	maxBridges := int(coresForBridges * BridgesPerCore)

	placesTaken := in.Admitted + in.WaitingAhead
	freePlaces := maxBridges - placesTaken

	d := Decision{Admit: freePlaces > 0, Limit: &maxBridges}
	if !d.Admit {
		d.Cause = BridgeLimit
		d.Reason = fmt.Sprintf("Cannot create bridge - limit exceeded (%d bridges maximum with %.1f CPU cores, %d core reserved for Redpanda)", maxBridges, cores, redpandaReservedCores)
	}

	return d
}

// limitCores returns the core count the bridge limit is computed from: the CPU
// limit when it is known and above zero, and the host's cores otherwise.
func limitCores(in Input) float64 {
	if in.CapacityCores != nil && *in.CapacityCores > 0 {
		return *in.CapacityCores
	}

	return float64(in.HostCores)
}
