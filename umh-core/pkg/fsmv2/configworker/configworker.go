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
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// Ref identifies a child spec by its worker type and name.
type Ref struct {
	WorkerType string
	Name       string
}

// Registry holds the child specs recorded by Upsert, keyed by Ref. Upsert is
// the only writer; readers go through Lookup or Snapshot, which return copies
// so no caller can mutate the stored specs or bypass the Enabled=true invariant.
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
// spec's UserSpec.Config.
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
