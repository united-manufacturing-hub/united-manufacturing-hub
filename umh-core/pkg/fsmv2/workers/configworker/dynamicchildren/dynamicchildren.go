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

// Package dynamicchildren owns the shared registry of child specs that the
// application control surface reads to spawn dynamic children.
//
// The registry is created by parent wiring at process scope and published via
// register.SetDeps (see WireSharedRegistry). Workers and clients hold handles
// only, so the registry survives worker restarts; no worker owns its lifetime.
package dynamicchildren

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

// Writer owns a shared Registry and records child specs via Upsert.
type Writer struct {
	registry *Registry
	validate func(config.ChildSpec) error
}

// Option configures a Writer at construction.
type Option func(*Writer)

// WithValidate installs a content-validation hook that Upsert runs against the
// built spec before recording it. A non-nil error from the hook makes Upsert
// reject the ref and leave the registry unchanged, preserving any spec
// previously recorded for ref.
func WithValidate(validate func(config.ChildSpec) error) Option {
	return func(w *Writer) {
		w.validate = validate
	}
}

// NewWriter returns a Writer with an empty registry.
func NewWriter(opts ...Option) *Writer {
	w := &Writer{
		registry: &Registry{specs: make(map[Ref]config.ChildSpec)},
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Registry returns the shared registry.
func (w *Writer) Registry() *Registry {
	return w.registry
}

// Upsert records an enabled child spec for ref, serializing cfg into the
// spec's UserSpec.Config. Upsert is the sole enforcer of the Enabled=true
// invariant on stored specs: every other writer must preserve it.
func (w *Writer) Upsert(ref Ref, cfg map[string]any) error {
	// TODO(ENG-5097): drop the YAML round-trip when ChildSpec.UserSpec.Config is removed (non-breaking)
	spec, err := config.NewChildSpec(ref.Name, ref.WorkerType, cfg, true)
	if err != nil {
		return err
	}

	if w.validate != nil {
		if err := w.validate(spec); err != nil {
			return err
		}
	}

	w.registry.mu.Lock()
	defer w.registry.mu.Unlock()
	w.registry.specs[ref] = spec
	return nil
}

// Delete removes the child spec recorded for ref from the shared registry.
func (w *Writer) Delete(ref Ref) {
	w.registry.mu.Lock()
	defer w.registry.mu.Unlock()
	delete(w.registry.specs, ref)
}
