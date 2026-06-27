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

package benthos_monitor

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// BenthosMonitorDependencies holds the base dependencies for the benthos
// monitor worker.
type BenthosMonitorDependencies struct {
	*deps.BaseDependencies
}

// NewBenthosMonitorDependencies creates dependencies for the benthos monitor
// worker. Accepts a shared BaseDependencies to avoid dual-instance metrics
// divergence.
func NewBenthosMonitorDependencies(baseDeps *deps.BaseDependencies) *BenthosMonitorDependencies {
	return &BenthosMonitorDependencies{
		BaseDependencies: baseDeps,
	}
}
