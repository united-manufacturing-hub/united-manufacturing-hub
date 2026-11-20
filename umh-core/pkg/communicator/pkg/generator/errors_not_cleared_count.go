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

// The not-cleared count of the status message's FeatureUsage.

package generator

import (
	"context"
	"time"

	fsmv2benthosmetrics "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
)

// notClearedMargin is the slack on top of the error window. An entry clears
// at most one scrape late, so a minute counts only entries that never cleared.
const notClearedMargin = time.Minute

// notClearedLookupTimeout bounds all store reads of one count. It is a small
// part of the status message's budget, so a slow store costs the count
// (unknown reads answer from the hold) and not the status message.
const notClearedLookupTimeout = 100 * time.Millisecond

// benthosErrorsNotClearedInstanceCount returns how many Benthos workers hold
// an error that should have cleared, or 0 with USE_FSMV2_BENTHOS_METRICS off.
func benthosErrorsNotClearedInstanceCount(ctx context.Context) int {
	useFsmv2, err := benthos.FSMv2BenthosMetricsEnabled()
	if err != nil || !useFsmv2 {
		return 0
	}

	ctx, cancel := context.WithTimeout(ctx, notClearedLookupTimeout)
	defer cancel()

	return fsmv2benthosmetrics.ErrorsNotClearedCount(ctx, notClearedMargin)
}
