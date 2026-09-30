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
// Decide is a pure function: it reads only its Input and has no logger, clock
// or system call, so both the FSMv1 protocol converter and an FSMv2 state can
// call it. The caller gathers the inputs from whatever it observes and acts on
// the Decision.
//
// A resource whose health nobody has shown is Unknown, and Unknown refuses.
// The only way to admit bridges without proven health is to turn bridge
// admission off with agent.enableResourceLimitBlocking: false.
package bridgeadmission

import "fmt"

// BridgesPerCore defines the maximum number of protocol converter bridges per CPU core.
// This limit ensures stable performance and prevents resource exhaustion.
// See docs/production/sizing-guide.md for more details on resource planning.
const BridgesPerCore = 5

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
	// BlockingEnabled is agent.enableResourceLimitBlocking. When false, every
	// bridge is admitted and nothing else is checked.
	BlockingEnabled bool

	CPU    Resource
	Memory Resource
	Disk   Resource

	// Admitted counts the bridges that already passed admission and are not
	// being removed. It excludes the bridge being decided.
	Admitted int
	// WaitingAhead counts the bridges that are still waiting for admission and
	// come before the bridge being decided in config.yaml. Bridges waiting
	// behind it are not counted, so after a restart the first bridges up to
	// the limit start and the rest wait.
	WaitingAhead int

	// CapacityCores is the CPU limit in cores, or nil when it is not known.
	CapacityCores *float64
	// HostCores is the number of cores on the host, used when CapacityCores is
	// nil.
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

// Decide returns whether a bridge may be created.
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

	cores := float64(in.HostCores)
	if in.CapacityCores != nil && *in.CapacityCores > 0 {
		cores = *in.CapacityCores
	}

	maxBridges := int(max(cores-1, 0) * BridgesPerCore)

	d := Decision{Admit: in.Admitted+in.WaitingAhead+1 <= maxBridges, Limit: &maxBridges}
	if !d.Admit {
		d.Cause = BridgeLimit
		d.Reason = fmt.Sprintf("Cannot create bridge - limit exceeded (%d bridges maximum with %.1f CPU cores, 1 core reserved for Redpanda)", maxBridges, cores)
	}

	return d
}
