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

package deps_test

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// Compile-time assertion: BaseDependencies must satisfy Dependencies.
// Ensures MetricsRecorder promotion to the interface does not break existing implementations.
var _ deps.Dependencies = (*deps.BaseDependencies)(nil)

func TestDepsInterfaceCompiles(t *testing.T) {
	t.Log("deps.Dependencies interface compile-time assertion passes")
}
