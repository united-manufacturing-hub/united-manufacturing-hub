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

package configworker

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestUpsertRecordsEnabledChildSpec verifies a ConfigWorker records a Ref into its
// shared registry as a config.ChildSpec that carries the structured config (serialized
// internally) and Enabled=true.
func TestUpsertRecordsEnabledChildSpec(t *testing.T) {
	cw := NewConfigWorker()

	ref := Ref{WorkerType: "example", Name: "foo"}
	cfg := map[string]any{"greeting": "hello"}

	if err := cw.Upsert(ref, cfg); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	spec, ok := cw.Registry().Lookup(ref)
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
	cw := NewConfigWorker()

	target := Ref{WorkerType: "example", Name: "foo"}
	survivor := Ref{WorkerType: "example", Name: "bar"}

	if err := cw.Upsert(target, map[string]any{"greeting": "hello"}); err != nil {
		t.Fatalf("Upsert target returned error: %v", err)
	}
	if err := cw.Upsert(survivor, map[string]any{"greeting": "hi"}); err != nil {
		t.Fatalf("Upsert survivor returned error: %v", err)
	}

	cw.Delete(target)

	if _, ok := cw.Registry().Lookup(target); ok {
		t.Errorf("registry still holds target %+v after Delete", target)
	}

	if _, ok := cw.Registry().Lookup(survivor); !ok {
		t.Errorf("registry dropped survivor %+v after deleting target", survivor)
	}

	if got := len(cw.Registry().Snapshot()); got != 1 {
		t.Errorf("registry has %d entries after Delete, want 1", got)
	}
}

// TestSpecsStableOrderAcrossReads verifies Specs returns the recorded child
// specs in a deterministic order (by WorkerType, then Name) on every call, even
// though the underlying map iteration is randomized. Without this, a reader that
// persists the specs would emit a spurious order-only delta on each read. The
// assertion checks the returned ChildSpecs' Names against the sorted order.
func TestSpecsStableOrderAcrossReads(t *testing.T) {
	cw := NewConfigWorker()

	refs := []Ref{
		{WorkerType: "zeta", Name: "b"},
		{WorkerType: "alpha", Name: "b"},
		{WorkerType: "alpha", Name: "a"},
		{WorkerType: "beta", Name: "a"},
	}
	for _, ref := range refs {
		if err := cw.Upsert(ref, map[string]any{"v": 1}); err != nil {
			t.Fatalf("Upsert(%+v) returned error: %v", ref, err)
		}
	}

	want := []Ref{
		{WorkerType: "alpha", Name: "a"},
		{WorkerType: "alpha", Name: "b"},
		{WorkerType: "beta", Name: "a"},
		{WorkerType: "zeta", Name: "b"},
	}

	for i := 0; i < 20; i++ {
		got := cw.Registry().Specs()
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

// TestUpsertReplacesOnSameRef exercises the "update" half of Upsert: a second
// Upsert with the same Ref overwrites the recorded spec rather than adding a
// duplicate, so the registry never spawns a stale spec.
func TestUpsertReplacesOnSameRef(t *testing.T) {
	cw := NewConfigWorker()

	ref := Ref{WorkerType: "example", Name: "foo"}

	if err := cw.Upsert(ref, map[string]any{"greeting": "hello"}); err != nil {
		t.Fatalf("first Upsert returned error: %v", err)
	}
	if err := cw.Upsert(ref, map[string]any{"greeting": "goodbye"}); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}

	snapshot := cw.Registry().Snapshot()
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
