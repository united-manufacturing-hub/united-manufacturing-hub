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

package main

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
)

// TestBenthosMonitorWorkerInitialStateRegistered guards the blank import of
// pkg/fsmv2/workers/benthos_monitor/state in main.go. The benthos_monitor
// worker's initial state is registered from that subpackage's init(); unlike
// the application worker, benthos_monitor/worker.go cannot blank-import its own
// state subpackage (an import cycle: state imports the parent for
// WorkerTypeName and the Config/Status types), so the registration must happen
// at the binary root. Without it the supervisor cannot instantiate a
// benthos_monitor child and GetFresh returns Unregistered for every bridge
// under USE_FSMV2_BENTHOS_MONITOR — the FF-on path would be a silent no-op.
// This test fails loudly if the main.go blank import is dropped.
func TestBenthosMonitorWorkerInitialStateRegistered(t *testing.T) {
	if fsmv2.LookupInitialState(benthos_monitor.WorkerTypeName) == nil {
		t.Fatalf("LookupInitialState(%q) = nil; the benthos_monitor initial state is not registered. "+
			"main.go must blank-import pkg/fsmv2/workers/benthos_monitor/state so its init() runs.",
			benthos_monitor.WorkerTypeName)
	}
}
