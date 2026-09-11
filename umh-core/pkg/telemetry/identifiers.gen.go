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

// Code generated from telemetry.yaml. DO NOT EDIT.

package telemetry

type cpuNode struct {
	ReadFailed Identifier
}

// Cpu holds the declared events of the cpu domain.
var Cpu = cpuNode{
	ReadFailed: Identifier{Tag: "cpu::read_failed", Brief: "A cgroup CPU file could not be read; the measurement continues.", Severity: SeverityWarning},
}

type telemetryNode struct {
	UnregisteredIdentifier Identifier
}

// Telemetry holds the declared events of the telemetry domain.
var Telemetry = telemetryNode{
	UnregisteredIdentifier: Identifier{Tag: "telemetry::unregistered_identifier", Brief: "A zero-value telemetry.Identifier reached the logger, so the call site is a bug.", Severity: SeverityError},
}

type transportPushNode struct {
	PersistentFailure Identifier
}

type transportNode struct {
	Push transportPushNode
}

// Transport holds the declared events of the transport domain.
var Transport = transportNode{
	Push: transportPushNode{
		PersistentFailure: Identifier{Tag: "transport::push::persistent_failure", Brief: "Outbound pushes keep failing; the instance may look offline.", Severity: SeverityError},
	},
}
