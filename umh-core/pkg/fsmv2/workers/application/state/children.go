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

package state

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// kernelChildName is the name of the config-worker kernel child that every
// alive application state emits once the shared registry is configured.
const kernelChildName = "config-worker"

// kernelWorkerType is the registered worker type the kernel child instantiates.
const kernelWorkerType = "configworker"

// renderUnion builds the additive union of the config-worker kernel (only when
// the registry is configured), the registry's dynamic children, and the worker's
// own declared children. Specs are deduped by Name with the FIRST occurrence
// winning, so the emit order encodes the Name-collision precedence
// kernel > registry > own: a kernel or registry child can never be shadowed by a
// same-named own child.
//
// Every emitted child is forced Enabled so the disable-mapping pass keeps it
// running. This deliberately overrides the Enabled bit a registry entry carries
// on snap.Status.DynamicChildren: a dynamic child cannot be authored as
// disabled-but-resident through here (mirroring DeriveDesiredState's passthrough
// model, which force-enables every YAML child). Stopping a child is gated
// elsewhere — via the disable-mapping pass / IsShutdownRequested — not via a
// stored Enabled=false. If disabled-but-resident dynamic children ever become a
// real need, honor snap.Status.DynamicChildren's Enabled here instead of
// overwriting it.
func renderUnion(snap fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]) []config.ChildSpec {
	union := make([]config.ChildSpec, 0, 1+len(snap.Status.DynamicChildren)+len(snap.ChildrenSpecs))

	if snap.Status.RegistryConfigured {
		union = append(union, config.ChildSpec{Name: kernelChildName, WorkerType: kernelWorkerType})
		union = append(union, snap.Status.DynamicChildren...)
	}

	union = append(union, snap.ChildrenSpecs...)

	// Empty union → nil, never an empty non-nil slice: a non-nil empty slice
	// signals the supervisor to despawn ALL children, while nil falls back to
	// the legacy ChildrenSpecs path.
	if len(union) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(union))
	deduped := make([]config.ChildSpec, 0, len(union))
	for _, spec := range union {
		if _, ok := seen[spec.Name]; ok {
			continue
		}
		seen[spec.Name] = struct{}{}
		spec.Enabled = true
		deduped = append(deduped, spec)
	}

	return deduped
}
