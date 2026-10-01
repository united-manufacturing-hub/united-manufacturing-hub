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

const emergencyHint = "In an emergency, you can start bridges anyway by setting agent.enableResourceLimitBlocking: false in the instance's Config File. It takes effect without a restart. Set it back to true once the resource problem is fixed."

// Health is what the caller knows about one resource.
type Health int

const (
	Unproven Health = iota
	Healthy
	Degraded
)

// Resource is the health of one resource and the message explaining it.
// Message may be empty. For Unproven it says why the health is not known.
type Resource struct {
	Health  Health
	Message string
}

type Input struct {
	EnableResourceLimitBlocking bool

	CPU    Resource
	Memory Resource
	Disk   Resource

	// Created counts the other bridges umh-core has already created, running
	// or stopped, and is not removing.
	Created int
	// WaitingBefore counts the bridges not created yet that config.yaml lists
	// before this one.
	WaitingBefore int

	// Cores is how many CPU cores the container may use: its CPU limit, or
	// every core of the host when it has none. Zero means not measured yet.
	Cores float64
}

// Cause names why a bridge was refused.
type Cause int

const (
	None Cause = iota
	NotProven
	CPU
	Memory
	Disk
	BridgeLimit
)

// Decision is the result of Decide.
type Decision struct {
	Admit  bool
	Cause  Cause
	Reason string
	// MaxBridges is how many bridges this instance may have, or nil when
	// Decide returned before computing it.
	MaxBridges *int
}

func (d Decision) Message() string {
	if d.Admit {
		return ""
	}

	return d.Reason + ". " + emergencyHint
}

// Decide returns whether a bridge may be created.
func Decide(in Input) Decision {
	if !in.EnableResourceLimitBlocking {
		return Decision{Admit: true}
	}

	for _, res := range in.resourcesInRefusalOrder() {
		if res.Health == Degraded {
			return refuse(res.cause, res.degradedReason())
		}
	}

	for _, res := range in.resourcesInRefusalOrder() {
		if res.Health == Unproven {
			return refuse(NotProven, res.unprovenReason())
		}
	}

	if in.Cores <= 0 {
		return refuse(NotProven, "Resource health not proven yet: CPU cores not measured yet")
	}

	maxBridges := int(max(in.Cores-redpandaReservedCores, 0) * BridgesPerCore)
	if in.Created+in.WaitingBefore >= maxBridges {
		d := refuse(BridgeLimit, fmt.Sprintf("Cannot create bridge - limit exceeded (%d bridges maximum with %.1f CPU cores, %d core reserved for Redpanda)", maxBridges, in.Cores, redpandaReservedCores))
		d.MaxBridges = &maxBridges

		return d
	}

	return Decision{Admit: true, MaxBridges: &maxBridges}
}

func refuse(cause Cause, reason string) Decision {
	return Decision{Cause: cause, Reason: reason}
}

type namedResource struct {
	Resource

	name  string
	cause Cause
}

func (in Input) resourcesInRefusalOrder() []namedResource {
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

func (r namedResource) unprovenReason() string {
	if r.Message == "" {
		return "Resource health not proven yet"
	}

	return "Resource health not proven yet: " + r.Message
}
