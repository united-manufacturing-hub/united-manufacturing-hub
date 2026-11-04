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

// The not-cleared count of the status message: how many Benthos instances
// the failure window still holds degraded after their errors should have
// cleared.

package generator

import (
	"context"
	"time"

	fsmv2benthos "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// notClearedMargin is the slack on top of ErrorWindow before an error entry
// counts as not cleared. An entry clears at most one scrape (1 s) late, so a
// minute counts only entries that did not clear at all.
const notClearedMargin = time.Minute

// notClearedLookupTimeout bounds the store reads the count makes: one read
// per registered Benthos worker, all under this one bound.
const notClearedLookupTimeout = time.Second

// benthosErrorsNotClearedInstanceCount counts the registered Benthos workers
// whose stored failures hold an error that should have cleared: an entry more
// than its error window plus notClearedMargin after its last error. It asks
// only with USE_FSMV2_BENTHOS_METRICS on, and is 0 with the flag off. The
// reads carry ctx, so the caller's deadline applies. The count is reporting
// only: it goes into the status message's FeatureUsage, and nothing in
// umh-core acts on it.
func benthosErrorsNotClearedInstanceCount(ctx context.Context) int {
	useFsmv2, err := benthos.FSMv2BenthosMetricsEnabled()
	if err != nil || !useFsmv2 {
		return 0
	}

	ctx, cancel := context.WithTimeout(ctx, notClearedLookupTimeout)
	defer cancel()

	return fsmv2benthos.ErrorsNotClearedCount(ctx, notClearedMargin)
}
