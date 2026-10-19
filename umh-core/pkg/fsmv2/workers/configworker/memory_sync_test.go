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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	fsmv2memory "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/memory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
)

func newMemoryConstructedWorker(t *testing.T, memoryEnabled bool) *configworker.ConfigworkerWorker {
	t.Helper()

	register.SetDeps[*dynamicchildren.Registry](workerType, dynamicchildren.NewWriter().Registry())
	t.Cleanup(func() { register.ClearDeps(workerType) })
	register.SetDeps[bool](configworker.MemoryMonitorEnabledDepsKey, memoryEnabled)
	t.Cleanup(func() { register.ClearDeps(configworker.MemoryMonitorEnabledDepsKey) })

	identity := deps.Identity{ID: workerType + "-001", WorkerType: workerType}

	w, err := configworker.NewConfigworkerWorker(identity, deps.NewNopFSMLogger(), nil)
	if err != nil {
		t.Fatalf("NewConfigworkerWorker: %v", err)
	}

	return w
}

func TestCollectObservedStateUpsertsMemoryWhenEnabled(t *testing.T) {
	w := newMemoryConstructedWorker(t, true)
	clientShared := withClient(t)

	desired := &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{}
	if _, err := w.CollectObservedState(context.Background(), desired); err != nil {
		t.Fatalf("CollectObservedState (memory enabled): %v", err)
	}

	if !clientShared.Contains(fsmv2memory.Ref) {
		t.Fatalf("memory enabled: registry does not contain %v after reconcile", fsmv2memory.Ref)
	}
}

func TestCollectObservedStateDoesNotUpsertMemoryWhenDisabled(t *testing.T) {
	w := newMemoryConstructedWorker(t, false)
	clientShared := withClient(t)

	desired := &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{}
	if _, err := w.CollectObservedState(context.Background(), desired); err != nil {
		t.Fatalf("CollectObservedState (memory disabled): %v", err)
	}

	if clientShared.Contains(fsmv2memory.Ref) {
		t.Fatalf("memory disabled: registry contains %v after reconcile, but the flag is off", fsmv2memory.Ref)
	}
}
