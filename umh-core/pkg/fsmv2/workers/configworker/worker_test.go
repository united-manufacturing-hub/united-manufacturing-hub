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

package configworker_test

import (
	"context"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	registry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	worker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
	state "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
)

// workerType is the canonical name registered in init() and used as the folder name.
const workerType = "configworker"

// TestConfigWorkerRegistersHoldsRegistryAndRunsHealthy verifies the three
// behaviors of the kernel config worker:
//
//  1. The worker type registers (the blank-import side-effect installs a factory
//     that factory.NewWorkerByType can resolve).
//  2. Its healthy state derives the state name "Running" (not "Connected").
//  3. A constructed worker holds the non-nil shared registry published via the
//     typed-deps wiring (register.SetDeps -> register.GetDeps).
func TestConfigWorkerRegistersHoldsRegistryAndRunsHealthy(t *testing.T) {
	// Behavior 2: the healthy state's derived name is "Running".
	if got := helpers.DeriveStateName(&state.RunningState{}); got != "Running" {
		t.Fatalf("healthy state DeriveStateName = %q, want %q", got, "Running")
	}

	// Publish the shared registry under the worker type so the constructor's
	// register.GetDeps wiring can pick it up; tear it down afterwards.
	shared := registry.NewConfigWorker().Registry()
	register.SetDeps[*registry.Registry](workerType, shared)
	t.Cleanup(func() { register.ClearDeps(workerType) })

	identity := deps.Identity{ID: workerType + "-001", WorkerType: workerType}
	logger := deps.NewNopFSMLogger()

	// Behavior 1: the registered factory resolves and constructs a worker.
	w, err := factory.NewWorkerByType(workerType, identity, logger, nil, nil)
	if err != nil {
		t.Fatalf("factory.NewWorkerByType(%q): %v", workerType, err)
	}

	if w == nil {
		t.Fatalf("factory.NewWorkerByType(%q) returned nil worker", workerType)
	}

	// Behavior 3: the constructed worker holds the non-nil shared registry handle,
	// and it is the same instance that was published.
	cw, ok := w.(*worker.ConfigworkerWorker)
	if !ok {
		t.Fatalf("worker is %T, want *configworker.ConfigworkerWorker", w)
	}

	if cw.Registry() == nil {
		t.Fatalf("constructed worker holds a nil registry handle")
	}

	if cw.Registry() != shared {
		t.Errorf("constructed worker holds a different registry instance than the one published")
	}
}

// TestNewConfigworkerWorkerFailsWithoutRegistry verifies the constructor's
// fail-loud contract: with no registry published under the worker type, it
// returns a non-nil error and a nil worker instead of a worker holding a nil
// registry handle.
func TestNewConfigworkerWorkerFailsWithoutRegistry(t *testing.T) {
	register.ClearDeps(workerType)

	identity := deps.Identity{ID: workerType + "-001", WorkerType: workerType}
	logger := deps.NewNopFSMLogger()

	w, err := worker.NewConfigworkerWorker(identity, logger, nil)
	if err == nil {
		t.Fatalf("NewConfigworkerWorker without a published registry: want error, got nil")
	}

	if w != nil {
		t.Fatalf("NewConfigworkerWorker without a published registry: want nil worker, got %v", w)
	}
}

// TestGetDependenciesAnyReturnsNil verifies the no-deps override returns a true
// nil so the framework skips metrics injection (a boxed register.NoDeps would
// be non-nil and silently re-enable it).
func TestGetDependenciesAnyReturnsNil(t *testing.T) {
	w := newConstructedWorker(t)

	if got := w.GetDependenciesAny(); got != nil {
		t.Fatalf("GetDependenciesAny() = %v, want nil", got)
	}
}

// TestCollectObservedState verifies the per-tick observed-state contract:
// a live context yields a non-nil observation of the worker's status type,
// and a cancelled context surfaces ctx.Err().
func TestCollectObservedState(t *testing.T) {
	w := newConstructedWorker(t)
	desired := &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{}

	obs, err := w.CollectObservedState(context.Background(), desired)
	if err != nil {
		t.Fatalf("CollectObservedState with live context: %v", err)
	}

	if _, ok := obs.(fsmv2.Observation[snapshot.ConfigworkerStatus]); !ok {
		t.Fatalf("CollectObservedState observation is %T, want fsmv2.Observation[snapshot.ConfigworkerStatus]", obs)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	obs, err = w.CollectObservedState(ctx, desired)
	if err != context.Canceled {
		t.Fatalf("CollectObservedState with cancelled context: err = %v, want %v", err, context.Canceled)
	}

	if obs != nil {
		t.Fatalf("CollectObservedState with cancelled context: want nil observation, got %v", obs)
	}
}

// newConstructedWorker publishes a registry, constructs a worker through it,
// and registers teardown so each test starts from a clean deps registry.
func newConstructedWorker(t *testing.T) *worker.ConfigworkerWorker {
	t.Helper()

	shared := registry.NewConfigWorker().Registry()
	register.SetDeps[*registry.Registry](workerType, shared)
	t.Cleanup(func() { register.ClearDeps(workerType) })

	identity := deps.Identity{ID: workerType + "-001", WorkerType: workerType}
	w, err := worker.NewConfigworkerWorker(identity, deps.NewNopFSMLogger(), nil)
	if err != nil {
		t.Fatalf("NewConfigworkerWorker: %v", err)
	}

	return w
}
