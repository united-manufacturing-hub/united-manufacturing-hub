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
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
)

// newCPUConstructedWorker publishes a registry and the CPU flag, constructs a
// config worker through it, and registers teardown. It returns the worker and
// the registry, mirroring newConstructedWorker but with a controllable flag.
func newCPUConstructedWorker(t *testing.T, cpuEnabled bool) (*configworker.ConfigworkerWorker, *dynamicchildren.Registry) {
	t.Helper()

	shared := dynamicchildren.NewWriter().Registry()
	register.SetDeps[*dynamicchildren.Registry](workerType, shared)
	t.Cleanup(func() { register.ClearDeps(workerType) })
	register.SetDeps[bool](configworker.CPUEnabledDepsKey, cpuEnabled)
	t.Cleanup(func() { register.ClearDeps(configworker.CPUEnabledDepsKey) })

	identity := deps.Identity{ID: workerType + "-001", WorkerType: workerType}

	w, err := configworker.NewConfigworkerWorker(identity, deps.NewNopFSMLogger(), nil)
	if err != nil {
		t.Fatalf("NewConfigworkerWorker: %v", err)
	}

	return w, shared
}

// withClient publishes a client backed by a fresh Writer and returns that
// Writer's registry. That registry is deliberately not the one
// newCPUConstructedWorker hands the worker.
// TestCollectObservedStateUpsertsCPUWhenEnabled therefore sees the child only
// when the worker went through the client. A reconcile that wrote its own
// registry directly would fail that test.
func withClient(t *testing.T) *dynamicchildren.Registry {
	t.Helper()

	dynWriter := dynamicchildren.NewWriter()
	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynWriter, nil))
	t.Cleanup(func() { fsmv2client.SetClient(nil) })

	return dynWriter.Registry()
}

func TestCollectObservedStateUpsertsCPUWhenEnabled(t *testing.T) {
	w, _ := newCPUConstructedWorker(t, true)
	clientShared := withClient(t)

	desired := &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{}
	if _, err := w.CollectObservedState(context.Background(), desired); err != nil {
		t.Fatalf("CollectObservedState (cpu enabled): %v", err)
	}

	if !clientShared.Contains(fsmv2cpu.Ref) {
		t.Fatalf("cpu enabled: registry does not contain %v after reconcile", fsmv2cpu.Ref)
	}
}

func TestCollectObservedStateDoesNotUpsertCPUWhenDisabled(t *testing.T) {
	w, _ := newCPUConstructedWorker(t, false)
	clientShared := withClient(t)

	desired := &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{}
	if _, err := w.CollectObservedState(context.Background(), desired); err != nil {
		t.Fatalf("CollectObservedState (cpu disabled): %v", err)
	}

	if clientShared.Contains(fsmv2cpu.Ref) {
		t.Fatalf("cpu disabled: registry contains %v after reconcile, but the flag is off", fsmv2cpu.Ref)
	}
}
