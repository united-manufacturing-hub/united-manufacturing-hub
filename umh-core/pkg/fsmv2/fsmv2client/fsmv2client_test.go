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

package fsmv2client

import (
	"context"
	"errors"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// TestUpsertAndDeletePassThroughToWriter verifies the FSMv2Client delegates
// writes to the Writer it wraps: Upsert records the ref in the underlying
// shared registry (Lookup ok==true) and Delete removes it (Lookup ok==false). The
// client is constructed with a nil StateReader because A11 only stores the reader
// for a later typed Get and does not read it here.
func TestUpsertAndDeletePassThroughToWriter(t *testing.T) {
	w := dynamicchildren.NewWriter()
	client := NewFSMv2Client(w, nil)

	ref := dynamicchildren.Ref{WorkerType: "example", Name: "foo"}
	cfg := map[string]any{"greeting": "hello"}

	if err := client.Upsert(ref, cfg); err != nil {
		t.Fatalf("client.Upsert returned error: %v", err)
	}

	if _, ok := w.Registry().Lookup(ref); !ok {
		t.Fatalf("Writer registry has no entry for ref %+v after client.Upsert", ref)
	}

	client.Delete(ref)

	if _, ok := w.Registry().Lookup(ref); ok {
		t.Fatalf("Writer registry still holds ref %+v after client.Delete", ref)
	}
}

// TestGetReturnsErrorOnNilStateReader verifies that Get on a write-only client
// (one built with a nil StateReader, a documented and used construction) returns
// an error instead of panicking on the nil dereference.
func TestGetReturnsErrorOnNilStateReader(t *testing.T) {
	w := dynamicchildren.NewWriter()
	client := NewFSMv2Client(w, nil)

	ref := dynamicchildren.Ref{WorkerType: "example", Name: "foo"}

	if _, err := Get[struct{}](context.Background(), client, ref); err == nil {
		t.Fatalf("Get on a nil-StateReader client returned nil error, want a non-nil error rather than a panic")
	}
}

func TestSetVariablesPassesThroughToWriter(t *testing.T) {
	w := dynamicchildren.NewWriter()
	client := NewFSMv2Client(w, nil)

	client.SetVariables(config.VariableBundle{User: map[string]any{"IP": "10.0.0.1"}})

	if got := w.Registry().Variables().User["IP"]; got != "10.0.0.1" {
		t.Fatalf("registry User[IP] = %v, want 10.0.0.1", got)
	}
}

type desiredTestConfig struct {
	Address string `json:"address"`
}

func newDesiredTestStore(t *testing.T, workerType string) *storage.TriangularStore {
	t.Helper()

	basic := memory.NewInMemoryStore()
	if err := basic.CreateCollection(context.Background(), workerType+"_desired", nil); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	return storage.NewTriangularStore(basic, deps.NewNopFSMLogger())
}

func TestGetDesiredReturnsTheSavedConfig(t *testing.T) {
	ctx := context.Background()
	ref := dynamicchildren.Ref{WorkerType: "examplechild", Name: "child-0"}
	store := newDesiredTestStore(t, ref.WorkerType)

	if _, err := store.SaveDesired(ctx, ref.WorkerType, config.ChildID(ref.Name), persistence.Document{
		"id":     config.ChildID(ref.Name),
		"config": map[string]any{"address": "192.168.1.100:502"},
	}); err != nil {
		t.Fatalf("SaveDesired: %v", err)
	}

	got, err := GetDesired[desiredTestConfig](ctx, NewFSMv2Client(dynamicchildren.NewWriter(), store), ref)
	if err != nil {
		t.Fatalf("GetDesired returned error: %v", err)
	}

	if got.Address != "192.168.1.100:502" {
		t.Fatalf("GetDesired Address = %q, want 192.168.1.100:502", got.Address)
	}
}

func TestGetDesiredReturnsErrNoDesiredStateWhenNothingIsSaved(t *testing.T) {
	ref := dynamicchildren.Ref{WorkerType: "examplechild", Name: "child-0"}
	client := NewFSMv2Client(dynamicchildren.NewWriter(), newDesiredTestStore(t, ref.WorkerType))

	if _, err := GetDesired[desiredTestConfig](context.Background(), client, ref); !errors.Is(err, ErrNoDesiredState) {
		t.Fatalf("GetDesired with nothing saved returned %v, want a wrapped ErrNoDesiredState", err)
	}
}
