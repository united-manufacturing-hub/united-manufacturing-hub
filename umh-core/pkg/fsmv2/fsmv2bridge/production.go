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

package fsmv2bridge

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// PublishForProduction stands up the FSMv2 child-observation bridge for
// production use. It creates the dynamicchildren Writer, publishes its Registry
// under the configworker deps key (so the application supervisor spawns the
// configworker kernel child and enables dynamic child spawning on the first
// tick), and publishes the process-scoped Client.
//
// The registry must be published before the application supervisor is
// constructed, so the application worker's first CollectObservedState observes
// RegistryConfigured=true and emits the configworker kernel child.
//
// The returned cleanup clears the Client and the deps key, and must run after
// the supervisor has stopped (mirroring runV2's "clear strictly after supDone"
// teardown ordering).
func PublishForProduction(sr deps.StateReader) func() {
	writer := dynamicchildren.NewWriter()
	register.SetDeps[*dynamicchildren.Registry](configworker.WorkerTypeName, writer.Registry())
	Set(New(writer, sr))

	return func() {
		Set(nil)
		register.ClearDeps(configworker.WorkerTypeName)
	}
}
