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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// WireSharedRegistry publishes the single reg instance under the application
// key and the config-worker key, so a write through one key's handle is visible
// through the other. Wiring both keys to one instance guards against the fork
// where the application key and the config-worker key hold two different
// registries.
func WireSharedRegistry(reg *Registry, applicationKey, configWorkerKey string) {
	register.SetDeps[*Registry](applicationKey, reg)
	register.SetDeps[*Registry](configWorkerKey, reg)
}
