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

package fsmv2client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	appsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"

	// Blank-import the kernel config worker plus the dynamic worker the registry
	// declares, so their init() registrations exist before the supervisor ticks.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

// TestGetReadsObservedStateWrittenByRealCollector proves that fsmv2client.Get[T]
// reads the observed state the REAL collector persists for a spawned child, and
// returns a not-found error (never a zero value) for a ref that was never
// observed.
//
// The harness mirrors the A7/A8 application-spawn integration path: one shared
// registry wired under the application and config-worker keys, a helloworld
// child Upserted into it, then the application supervisor driven through its
// real tick loop (TestMarkAsStarted + TestTick) until the collector has
// persisted the child's observed state under worker type "helloworld" and id
// "hello-1-001". The client holds the same TriangularStore as its read-only
// StateReader, so Get reads exactly what the collector wrote.
//
// The read re-ticks and re-Gets until the collector's own cadence persists
// "Running"; it does not rely on any synchronous post-transition collect.
func TestGetReadsObservedStateWrittenByRealCollector(t *testing.T) {
	g := gomega.NewWithT(t)

	const (
		appKey          = "application"
		configWorkerKey = "configworker"
	)

	ctx := context.Background()
	logger := deps.NewNopFSMLogger()

	t.Cleanup(func() {
		register.ClearDeps(appKey)
		register.ClearDeps(configWorkerKey)
	})

	// One shared registry, wired under both keys before the application worker is
	// constructed, so the COS read sees a non-nil handle.
	cw := configworker.NewConfigWorker()
	configworker.WireSharedRegistry(cw.Registry(), appKey, configWorkerKey)

	// Upsert a helloworld child. Empty MoodFilePath means the worker never goes
	// "sad", so it deterministically reaches Running.
	childRef := configworker.Ref{WorkerType: "helloworld", Name: "hello-1"}
	if err := cw.Upsert(childRef, map[string]any{"state": "running"}); err != nil {
		t.Fatalf("Upsert helloworld child: %v", err)
	}

	// Build the application supervisor through the boot path and keep the store
	// it writes observed state into.
	const appID = "test-app-001"
	basicStore := memory.NewInMemoryStore()

	appWorkerType, err := storage.DeriveWorkerType[fsmv2.Observation[appsnapshot.ApplicationStatus]]()
	if err != nil {
		t.Fatalf("DeriveWorkerType: %v", err)
	}
	_ = basicStore.CreateCollection(ctx, appWorkerType+"_identity", nil)
	_ = basicStore.CreateCollection(ctx, appWorkerType+"_desired", nil)
	_ = basicStore.CreateCollection(ctx, appWorkerType+"_observed", nil)

	store := storage.NewTriangularStore(basicStore, logger)

	sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           appID,
		Name:         "Test Application Supervisor",
		Store:        store,
		Logger:       logger,
		TickInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewApplicationSupervisor: %v", err)
	}

	// Mark started so the spawned child is handed the supervisor's long-lived
	// context and its collector runs -- the same started==true path production
	// takes after Start()/StartAsChild().
	sup.TestMarkAsStarted()

	// The client holds the same store as a read-only StateReader.
	client := fsmv2client.NewFSMv2Client(cw, store)

	// Re-tick and re-Get until the collector's own cadence has persisted
	// "Running" for the spawned child. Get reads exactly what the collector
	// wrote, so this proves the read-back without any synchronous collect.
	g.Eventually(func() string {
		_ = sup.TestTick(ctx)

		obs, getErr := fsmv2client.Get[hello_world.HelloworldStatus](ctx, client, childRef)
		if getErr != nil {
			return ""
		}

		return obs.State
	}, "15s", "100ms").Should(gomega.Equal("Running"),
		"helloworld child's observed state never reached Running through the real collector cadence")

	// Get for an undeclared ref (never Upserted, never spawned) must return a
	// not-found error -- never a zero value masquerading as data.
	undeclaredRef := configworker.Ref{WorkerType: "helloworld", Name: "never-spawned"}
	_, err = fsmv2client.Get[hello_world.HelloworldStatus](ctx, client, undeclaredRef)
	if !errors.Is(err, fsmv2client.ErrNotObserved) {
		t.Fatalf("Get for undeclared ref returned %v; want a wrapped ErrNotObserved, not a zero value", err)
	}
}
