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

// Package configworker owns the shared registry of child specs that the
// application control surface reads to spawn dynamic children.
package configworker

import (
	"sort"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// Ref identifies a child spec by its worker type and name, and is the registry
// map key for Upsert, Delete, and Lookup. The JSON tags keep the snake_case
// convention used across the CSE snapshot.
type Ref struct {
	WorkerType string `json:"worker_type"`
	Name       string `json:"name"`
}

// Registry holds the child specs recorded by Upsert, keyed by Ref. Upsert and
// Delete are the only writers; readers go through Lookup or Snapshot, which
// return copies so no caller can mutate the stored specs.
type Registry struct {
	specs map[Ref]config.ChildSpec
	mu    sync.RWMutex
}

// Lookup returns a copy of the spec recorded for ref, and whether it exists.
func (r *Registry) Lookup(ref Ref) (config.ChildSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	spec, ok := r.specs[ref]
	if !ok {
		return config.ChildSpec{}, false
	}
	return spec.Clone(), true
}

// Snapshot returns a copy of the registry's specs, keyed by Ref.
func (r *Registry) Snapshot() map[Ref]config.ChildSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[Ref]config.ChildSpec, len(r.specs))
	for ref, spec := range r.specs {
		out[ref] = spec.Clone()
	}
	return out
}

// Specs returns clones of the recorded child specs in a stable order (by
// WorkerType, then Name). Map iteration over the registry is randomized per
// call, so readers that persist the specs (e.g. into a CSE snapshot) must use
// this accessor to avoid spurious order-only deltas on every read. The clones
// keep callers from mutating the stored specs, and each carries the full spec
// (Name, WorkerType, UserSpec.Config, Enabled) so the reader can spawn the
// child, which a Ref (WorkerType+Name only) cannot.
func (r *Registry) Specs() []config.ChildSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	specs := make([]config.ChildSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec.Clone())
	}

	sort.Slice(specs, func(i, j int) bool {
		if specs[i].WorkerType != specs[j].WorkerType {
			return specs[i].WorkerType < specs[j].WorkerType
		}
		return specs[i].Name < specs[j].Name
	})

	return specs
}

// ConfigWorker owns a shared Registry and records child specs via Upsert.
type ConfigWorker struct {
	registry *Registry
}

// NewConfigWorker returns a ConfigWorker with an empty registry.
func NewConfigWorker() *ConfigWorker {
	return &ConfigWorker{
		registry: &Registry{specs: make(map[Ref]config.ChildSpec)},
	}
}

// Registry returns the shared registry.
func (cw *ConfigWorker) Registry() *Registry {
	return cw.registry
}

// Upsert records an enabled child spec for ref, serializing cfg into the
// spec's UserSpec.Config. Upsert is the sole enforcer of the Enabled=true
// invariant on stored specs: every other writer must preserve it.
func (cw *ConfigWorker) Upsert(ref Ref, cfg map[string]any) error {
	// TODO(ENG-5097): drop the YAML round-trip when ChildSpec.UserSpec.Config is removed (non-breaking)
	spec, err := config.NewChildSpec(ref.Name, ref.WorkerType, cfg, true)
	if err != nil {
		return err
	}

	cw.registry.mu.Lock()
	defer cw.registry.mu.Unlock()
	cw.registry.specs[ref] = spec
	return nil
}

// Delete removes the child spec recorded for ref from the shared registry.
func (cw *ConfigWorker) Delete(ref Ref) {
	cw.registry.mu.Lock()
	defer cw.registry.mu.Unlock()
	delete(cw.registry.specs, ref)
}
