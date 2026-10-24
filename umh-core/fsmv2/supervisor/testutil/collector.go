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

//go:build test

package testutil

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor/internal/collection"
)

// CollectorConfig is the supervisor's collector configuration, exposed for
// tests outside this module.
type CollectorConfig[TObserved fsmv2.ObservedState] = collection.CollectorConfig[TObserved]

// NewCollector returns the supervisor's collector for tests outside the fsmv2 module. Go's internal-directory rule stops those tests from importing supervisor/internal/collection directly.
func NewCollector[TObserved fsmv2.ObservedState](cfg CollectorConfig[TObserved]) *collection.Collector[TObserved] {
	return collection.NewCollector[TObserved](cfg)
}
