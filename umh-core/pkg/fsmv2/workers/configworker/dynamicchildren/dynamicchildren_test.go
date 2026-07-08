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

package dynamicchildren

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// TestUpsertRecordsEnabledChildSpec verifies a Writer records a Ref into its
// shared registry as a config.ChildSpec that carries the structured config (serialized
// internally) and Enabled=true.
func TestUpsertRecordsEnabledChildSpec(t *testing.T) {
	w := NewWriter()

	ref := Ref{WorkerType: "example", Name: "foo"}
	cfg := map[string]any{"greeting": "hello"}

	if err := w.Upsert(ref, cfg); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	spec, ok := w.Registry().Lookup(ref)
	if !ok {
		t.Fatalf("registry has no entry for ref %+v", ref)
	}

	if spec.Name != "foo" {
		t.Errorf("spec.Name = %q, want %q", spec.Name, "foo")
	}

	if spec.WorkerType != "example" {
		t.Errorf("spec.WorkerType = %q, want %q", spec.WorkerType, "example")
	}

	if !spec.Enabled {
		t.Errorf("spec.Enabled = %v, want true", spec.Enabled)
	}

	var got map[string]any
	if err := yaml.Unmarshal([]byte(spec.UserSpec.Config), &got); err != nil {
		t.Fatalf("UserSpec.Config is not valid YAML: %v", err)
	}
	if got["greeting"] != "hello" {
		t.Errorf("UserSpec.Config greeting = %v, want %q", got["greeting"], "hello")
	}
}

// TestDeleteRemovesRef verifies Delete removes only the targeted Ref and leaves
// the survivor in the registry (Lookup(survivor) still returns ok), so deleting
// one child does not drop specs the application control surface still needs.
func TestDeleteRemovesRef(t *testing.T) {
	w := NewWriter()

	target := Ref{WorkerType: "example", Name: "foo"}
	survivor := Ref{WorkerType: "example", Name: "bar"}

	if err := w.Upsert(target, map[string]any{"greeting": "hello"}); err != nil {
		t.Fatalf("Upsert target returned error: %v", err)
	}
	if err := w.Upsert(survivor, map[string]any{"greeting": "hi"}); err != nil {
		t.Fatalf("Upsert survivor returned error: %v", err)
	}

	w.Delete(target)

	if _, ok := w.Registry().Lookup(target); ok {
		t.Errorf("registry still holds target %+v after Delete", target)
	}

	if _, ok := w.Registry().Lookup(survivor); !ok {
		t.Errorf("registry dropped survivor %+v after deleting target", survivor)
	}

	if got := len(w.Registry().Snapshot()); got != 1 {
		t.Errorf("registry has %d entries after Delete, want 1", got)
	}
}

// TestSpecsStableOrderAcrossReads verifies Specs returns the recorded child
// specs in a deterministic order (by WorkerType, then Name) on every call, even
// though the underlying map iteration is randomized. Without this, a reader that
// persists the specs would emit a spurious order-only delta on each read. The
// assertion checks the returned ChildSpecs' Names against the sorted order.
func TestSpecsStableOrderAcrossReads(t *testing.T) {
	w := NewWriter()

	// Names are globally unique (Upsert rejects a Name reused across worker types);
	// alpha holds two names so the secondary Name sort is still exercised.
	refs := []Ref{
		{WorkerType: "zeta", Name: "z1"},
		{WorkerType: "alpha", Name: "a2"},
		{WorkerType: "alpha", Name: "a1"},
		{WorkerType: "beta", Name: "b1"},
	}
	for _, ref := range refs {
		if err := w.Upsert(ref, map[string]any{"v": 1}); err != nil {
			t.Fatalf("Upsert(%+v) returned error: %v", ref, err)
		}
	}

	want := []Ref{
		{WorkerType: "alpha", Name: "a1"},
		{WorkerType: "alpha", Name: "a2"},
		{WorkerType: "beta", Name: "b1"},
		{WorkerType: "zeta", Name: "z1"},
	}

	for i := 0; i < 20; i++ {
		got := w.Registry().Specs()
		if len(got) != len(want) {
			t.Fatalf("Specs() returned %d specs, want %d", len(got), len(want))
		}
		for j := range want {
			if got[j].WorkerType != want[j].WorkerType || got[j].Name != want[j].Name {
				t.Fatalf("Specs()[%d] = {WorkerType:%q, Name:%q}, want {WorkerType:%q, Name:%q} (read %d)",
					j, got[j].WorkerType, got[j].Name, want[j].WorkerType, want[j].Name, i)
			}
		}
	}
}

// TestUpsertValidationHookGatesRecording verifies the content-validation hook
// (an ENG-4900 stub) gates Upsert: a hook that returns a non-nil error makes
// Upsert reject the ref and leave the registry unchanged (Lookup ok==false),
// while a nil/passing hook admits the ref as before (Lookup ok==true,
// Enabled==true).
func TestUpsertValidationHookGatesRecording(t *testing.T) {
	ref := Ref{WorkerType: "example", Name: "foo"}
	cfg := map[string]any{"greeting": "hello"}

	t.Run("rejecting hook leaves registry unchanged", func(t *testing.T) {
		wantErr := errors.New("rejected by validation")
		w := NewWriter(WithValidate(func(config.ChildSpec) error {
			return wantErr
		}))

		err := w.Upsert(ref, cfg)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Upsert error = %v, want %v", err, wantErr)
		}

		if _, ok := w.Registry().Lookup(ref); ok {
			t.Errorf("registry recorded ref %+v despite rejecting hook", ref)
		}
	})

	t.Run("nil hook admits the ref", func(t *testing.T) {
		w := NewWriter()

		if err := w.Upsert(ref, cfg); err != nil {
			t.Fatalf("Upsert returned error: %v", err)
		}

		spec, ok := w.Registry().Lookup(ref)
		if !ok {
			t.Fatalf("registry has no entry for ref %+v", ref)
		}
		if !spec.Enabled {
			t.Errorf("spec.Enabled = %v, want true", spec.Enabled)
		}
	})

	t.Run("passing hook admits the post-build spec", func(t *testing.T) {
		var seen config.ChildSpec
		w := NewWriter(WithValidate(func(spec config.ChildSpec) error {
			seen = spec
			return nil
		}))

		if err := w.Upsert(ref, cfg); err != nil {
			t.Fatalf("Upsert returned error: %v", err)
		}

		if seen.Name != "foo" {
			t.Errorf("hook saw spec.Name = %q, want %q", seen.Name, "foo")
		}
		if seen.WorkerType != "example" {
			t.Errorf("hook saw spec.WorkerType = %q, want %q", seen.WorkerType, "example")
		}
		if !seen.Enabled {
			t.Errorf("hook saw spec.Enabled = %v, want true", seen.Enabled)
		}
		var hookCfg map[string]any
		if err := yaml.Unmarshal([]byte(seen.UserSpec.Config), &hookCfg); err != nil {
			t.Fatalf("hook saw UserSpec.Config that is not valid YAML: %v", err)
		}
		if hookCfg["greeting"] != "hello" {
			t.Errorf("hook saw UserSpec.Config greeting = %v, want %q", hookCfg["greeting"], "hello")
		}

		spec, ok := w.Registry().Lookup(ref)
		if !ok {
			t.Fatalf("registry has no entry for ref %+v", ref)
		}
		if !spec.Enabled {
			t.Errorf("spec.Enabled = %v, want true", spec.Enabled)
		}
	})

	t.Run("rejecting hook leaves the prior spec intact", func(t *testing.T) {
		wantErr := errors.New("rejected by validation")
		seeded := false
		w := NewWriter(WithValidate(func(config.ChildSpec) error {
			if !seeded {
				seeded = true
				return nil
			}
			return wantErr
		}))

		if err := w.Upsert(ref, map[string]any{"greeting": "hello"}); err != nil {
			t.Fatalf("first Upsert returned error: %v", err)
		}

		if err := w.Upsert(ref, map[string]any{"greeting": "goodbye"}); !errors.Is(err, wantErr) {
			t.Fatalf("second Upsert error = %v, want %v", err, wantErr)
		}

		spec, ok := w.Registry().Lookup(ref)
		if !ok {
			t.Fatalf("registry dropped ref %+v after rejected update", ref)
		}
		if !spec.Enabled {
			t.Errorf("spec.Enabled = %v, want true", spec.Enabled)
		}
		var got map[string]any
		if err := yaml.Unmarshal([]byte(spec.UserSpec.Config), &got); err != nil {
			t.Fatalf("UserSpec.Config is not valid YAML: %v", err)
		}
		if got["greeting"] != "hello" {
			t.Errorf("UserSpec.Config greeting = %v, want %q (prior spec must survive rejection)", got["greeting"], "hello")
		}
	})
}

// TestUpsertReplacesOnSameRef exercises the "update" half of Upsert: a second
// Upsert with the same Ref overwrites the recorded spec rather than adding a
// duplicate, so the registry never spawns a stale spec.
func TestUpsertReplacesOnSameRef(t *testing.T) {
	w := NewWriter()

	ref := Ref{WorkerType: "example", Name: "foo"}

	if err := w.Upsert(ref, map[string]any{"greeting": "hello"}); err != nil {
		t.Fatalf("first Upsert returned error: %v", err)
	}
	if err := w.Upsert(ref, map[string]any{"greeting": "goodbye"}); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}

	snapshot := w.Registry().Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("registry has %d entries, want 1", len(snapshot))
	}

	spec, ok := snapshot[ref]
	if !ok {
		t.Fatalf("registry has no entry for ref %+v", ref)
	}

	var got map[string]any
	if err := yaml.Unmarshal([]byte(spec.UserSpec.Config), &got); err != nil {
		t.Fatalf("UserSpec.Config is not valid YAML: %v", err)
	}
	if got["greeting"] != "goodbye" {
		t.Errorf("UserSpec.Config greeting = %v, want %q", got["greeting"], "goodbye")
	}
}

// TestUpsertRejectsNameReuseAcrossWorkerTypes guards the identity mismatch
// between this registry and the supervisor: the registry keys specs on the full
// Ref (WorkerType+Name), but the supervisor keys its children on Name alone, so
// two worker types sharing a Name store cleanly here yet can never reconcile.
// Upsert must reject the second write at the call site rather than let the
// collision surface later inside the parent's reconcile loop.
func TestUpsertRejectsNameReuseAcrossWorkerTypes(t *testing.T) {
	w := NewWriter()

	held := Ref{WorkerType: "example", Name: "foo"}
	if err := w.Upsert(held, map[string]any{"greeting": "hello"}); err != nil {
		t.Fatalf("first Upsert returned error: %v", err)
	}

	clash := Ref{WorkerType: "other", Name: "foo"}
	if err := w.Upsert(clash, map[string]any{"greeting": "hi"}); err == nil {
		t.Fatalf("Upsert(%+v) returned nil, want error: name %q already held under worker type %q", clash, clash.Name, held.WorkerType)
	}

	snapshot := w.Registry().Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("registry has %d entries, want 1 (the clashing spec must not be stored)", len(snapshot))
	}
	if _, ok := snapshot[held]; !ok {
		t.Fatalf("registry dropped the originally held ref %+v", held)
	}

	// A repeat write of the same Ref stays idempotent — the guard rejects only a
	// Name reused under a different worker type, never a re-Upsert of the same Ref.
	if err := w.Upsert(held, map[string]any{"greeting": "again"}); err != nil {
		t.Fatalf("idempotent re-Upsert of held ref returned error: %v", err)
	}
}
