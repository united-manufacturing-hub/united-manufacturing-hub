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

// TestProductionImportRegistersBenthosMonitorWorker fails if pkg/service/benthos
// stops importing the fsmv2 benthos_monitor package. Without that import,
// USE_FSMV2_BENTHOS_MONITOR=true selects a worker type that is not registered.
// The backend-selection specs in benthos_backend_test.go cover the constructor
// call.
func TestProductionImportRegistersBenthosMonitorWorker(t *testing.T) {
	const workerType = "benthos_monitor"

	registered := factory.ListRegisteredTypes()
	for _, wt := range registered {
		if wt == workerType {
			return
		}
	}

	t.Fatalf("worker type %q is not registered; registration must arrive through the production import in pkg/service/benthos, got %v", workerType, registered)
}
