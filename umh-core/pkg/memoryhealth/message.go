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

package memoryhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

const (
	messageNotMeasured = "Memory usage not measured yet"
	messageNormal      = "Memory utilization normal"
	messageWarning     = "Memory utilization warning"
	messageCritical    = "Memory utilization critical"

	warningFraction = constants.MemoryMediumThresholdPercent / 100
)

// ComposeMessage renders a verdict as the memory health message.
func ComposeMessage(verdict Verdict, details Details) string {
	if verdict.State == StateDegraded {
		return messageCritical
	}

	usedFraction, measured := details.AvgUsedFraction.Get()
	if !measured {
		return messageNotMeasured
	}

	if usedFraction >= warningFraction {
		return messageWarning
	}

	return messageNormal
}
