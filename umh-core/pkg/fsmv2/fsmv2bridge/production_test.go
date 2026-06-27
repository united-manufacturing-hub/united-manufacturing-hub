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

package fsmv2bridge_test

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2bridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// TestPublishForProduction_PublishesAndClears asserts the production wiring:
// PublishForProduction publishes the dynamicchildren registry under the
// configworker deps key and the process-scoped Client, and the returned
// cleanup clears both.
func TestPublishForProduction_PublishesAndClears(t *testing.T) {
	// Start clean: a prior spec may have left the process-global state dirty.
	fsmv2bridge.Set(nil)
	register.ClearDeps(configworker.WorkerTypeName)

	cleanup := fsmv2bridge.PublishForProduction(&stubStateReader{})

	if reg := register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName); reg == nil {
		t.Fatalf("configworker deps key not published")
	}

	if fsmv2bridge.Get() == nil {
		t.Fatalf("fsmv2bridge Client not published")
	}

	cleanup()

	if reg := register.GetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName); reg != nil {
		t.Fatalf("configworker deps key not cleared by cleanup")
	}

	if fsmv2bridge.Get() != nil {
		t.Fatalf("fsmv2bridge Client not cleared by cleanup")
	}
}
