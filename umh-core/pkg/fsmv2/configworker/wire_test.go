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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// TestWireSharedRegistryPublishesOneInstanceAcrossKeys verifies that
// WireSharedRegistry publishes a SINGLE *Registry instance under every supplied
// dependency key, so a write through one key's handle is visible through the
// other key's handle. This guards against the production-wiring bug where the
// application key and the config-worker key end up holding two DIFFERENT
// registries: the config worker writes one, the application reads the other
// (empty), and the fork only surfaces at the e2e capstone.
//
// The two keys are the worker-type strings from the register.Worker[...]
// registrations: "application" (workers/application, WorkerTypeName) and
// "configworker" (workers/configworker, workerType).
func TestWireSharedRegistryPublishesOneInstanceAcrossKeys(t *testing.T) {
	const (
		applicationKey  = "application"
		configWorkerKey = "configworker"
	)

	t.Cleanup(func() {
		register.ClearDeps(applicationKey)
		register.ClearDeps(configWorkerKey)
	})

	// One ConfigWorker owns one registry; publish that single instance under
	// both keys.
	cw := NewConfigWorker()
	reg := cw.Registry()

	WireSharedRegistry(reg, applicationKey, configWorkerKey)

	// Write through the config-worker-owned handle.
	ref := Ref{WorkerType: "example", Name: "foo"}
	if err := cw.Upsert(ref, map[string]any{"greeting": "hello"}); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	// Read through the OTHER key's published handle. The write must be visible
	// because it is the same instance.
	got := register.GetDeps[*Registry](applicationKey)
	if got == nil {
		t.Fatalf("GetDeps(%q) returned nil; registry not published under that key", applicationKey)
	}

	if _, ok := got.Lookup(ref); !ok {
		t.Errorf("write through the config-worker handle is not visible through the %q handle: forked registry instances", applicationKey)
	}
}
