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

// This file is deliberately in the external test package benthos_test and
// deliberately does not import the fsmv2 worker package itself: the only path
// by which the worker type can reach the factory registry here is the
// production import inside pkg/service/benthos. The worker type is spelled as a
// literal for the same reason — naming the package constant would pull the
// worker package in and register it from the test binary.
package benthos_test

import (
	"testing"

	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

// TestProductionImportRegistersBenthosMonitorWorker guards one link: the
// production import that registers the worker type. Drop it and
// USE_FSMV2_BENTHOS_MONITOR selects a backend that registers nothing, while a CPU
// measurement still reports a win because no monitor processes get spawned either
// way. It does NOT guard the FF-on constructor call — removing that call leaves
// this test green, because the import alone registers; the backend-selection
// specs in benthos_backend_test.go cover the call.
func TestProductionImportRegistersBenthosMonitorWorker(t *testing.T) {
	const workerType = "benthos_monitor"

	// benthos_monitor is the only type this test binary's import graph registers,
	// so an empty registry is the exact shape of the failure this guards against;
	// the message prints the whole list rather than checking emptiness separately.
	registered := factory.ListRegisteredTypes()
	for _, wt := range registered {
		if wt == workerType {
			return
		}
	}

	t.Fatalf("worker type %q is not registered; registration must arrive through the production import in pkg/service/benthos, got %v", workerType, registered)
}
