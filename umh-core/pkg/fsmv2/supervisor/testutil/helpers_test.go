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

package testutil_test

import (
	"context"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/testutil"
)

func TestWorkerCollectObservedState(t *testing.T) {
	worker := &testutil.Worker{}
	ctx := context.Background()

	observed, err := worker.CollectObservedState(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if observed == nil {
		t.Fatal("expected non-nil observed state")
	}

	state, ok := observed.(*testutil.ObservedState)
	if !ok {
		t.Fatalf("expected *testutil.ObservedState, got %T", observed)
	}

	if state.ID != "test-worker" {
		t.Errorf("expected ID 'test-worker', got %s", state.ID)
	}
}

func TestCreateTriangularStore(t *testing.T) {
	store := testutil.CreateTriangularStore()
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestCreateTriangularStoreForWorkerType(t *testing.T) {
	store := testutil.CreateTriangularStoreForWorkerType("s6")
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestIdentity(t *testing.T) {
	identity := testutil.Identity()

	if identity.ID != "test-worker" {
		t.Errorf("expected ID 'test-worker', got %s", identity.ID)
	}

	if identity.WorkerType != "container" {
		t.Errorf("expected WorkerType 'container', got %s", identity.WorkerType)
	}
}

func TestWorkerWithType(t *testing.T) {
	worker := &testutil.WorkerWithType{
		WorkerType: "s6",
	}
	ctx := context.Background()

	observed, err := worker.CollectObservedState(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	state, ok := observed.(*testutil.ObservedState)
	if !ok {
		t.Fatalf("expected *testutil.ObservedState, got %T", observed)
	}

	if state.ID != "s6-worker" {
		t.Errorf("expected ID 's6-worker', got %s", state.ID)
	}
}
