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
// A bridge is admitted only when CPU, memory and disk are proven healthy and a
// bridge fits under the maximum number of bridges. The one exception is the emergency
// switch agent.enableResourceLimitBlocking: false, which admits every bridge.
// Decide applies the rules top to bottom.
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

	// Created counts the other bridges umh-core has already created, running
	// or stopped, and is not removing.
	Created int
	// WaitingBefore counts the bridges not created yet that config.yaml lists
	// before this one. They are created first, so they count like Created.
	WaitingBefore int

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
	// MaxBridges is how many bridges this instance may have, or nil when
	// Decide refused before computing it.
	MaxBridges *int
}

// Message returns the text shown for a refused bridge: Reason followed by how
// to start bridges anyway. It returns "" for an admitted bridge.
func (d Decision) Message() string {
	if d.Admit {
		return ""
	}

	return d.Reason + ". " + emergencyHint
}

// Decide returns whether a bridge may be created.
func Decide(in Input) Decision {
	if !in.BlockingEnabled {
		return Decision{Admit: true}
	}

	for _, res := range in.resources() {
		if res.Health == Degraded {
			return refuse(res.cause, res.degradedReason())
		}
	}

	for _, res := range in.resources() {
		if res.Health == Unknown {
			return refuse(NotProven, res.unknownReason())
		}
	}

	maxBridges, cores := maxBridgesFor(in)
	if in.Created+in.WaitingBefore >= maxBridges {
		d := refuse(BridgeLimit, fmt.Sprintf("Cannot create bridge - limit exceeded (%d bridges maximum with %.1f CPU cores, %d core reserved for Redpanda)", maxBridges, cores, redpandaReservedCores))
		d.MaxBridges = &maxBridges

		return d
	}

	return Decision{Admit: true, MaxBridges: &maxBridges}
}

// maxBridgesFor returns how many bridges the instance may have, (cores - 1) * 5,
// and the cores it used: the CPU limit when known, else the host's cores.
func maxBridgesFor(in Input) (maxBridges int, cores float64) {
	cores = float64(in.HostCores)
	if in.CapacityCores != nil && *in.CapacityCores > 0 {
		cores = *in.CapacityCores
	}

	return int(max(cores-redpandaReservedCores, 0) * BridgesPerCore), cores
}

func refuse(cause Cause, reason string) Decision {
	return Decision{Cause: cause, Reason: reason}
}

type namedResource struct {
	Resource

	name  string
	cause Cause
}

// resources lists CPU, memory and disk in the order a refusal names them.
func (in Input) resources() []namedResource {
	return []namedResource{
		{Resource: in.CPU, name: "CPU", cause: CPU},
		{Resource: in.Memory, name: "Memory", cause: Memory},
		{Resource: in.Disk, name: "Disk", cause: Disk},
	}
}

func (r namedResource) degradedReason() string {
	if r.Message == "" {
		return r.name + " resources degraded"
	}

	return r.name + " degraded: " + r.Message
}

func (r namedResource) unknownReason() string {
	if r.Message == "" {
		return "Resource health not proven yet"
	}

	return "Resource health not proven yet: " + r.Message
}
