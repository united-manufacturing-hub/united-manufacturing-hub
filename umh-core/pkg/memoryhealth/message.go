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

	messageHostMemoryFull = "Machine memory nearly full"
	messagePressure       = "Processes are waiting for memory"
	messageOomKill        = "A process was stopped because memory ran out"

	warningFraction = constants.MemoryMediumThresholdPercent / 100
)

var degradedMessages = map[CauseKind]string{
	CauseKindContainerLimitFull: messageCritical,
	CauseKindHostMemoryFull:     messageHostMemoryFull,
	CauseKindPressure:           messagePressure,
	CauseKindOomKill:            messageOomKill,
}

// ComposeMessage renders a verdict as the memory health message.
func ComposeMessage(verdict Verdict, details Details) string {
	if verdict.State == StateDegraded {
		return degradedMessage(verdict)
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

func degradedMessage(verdict Verdict) string {
	if len(verdict.Causes) == 0 {
		return messageCritical
	}

	message, known := degradedMessages[verdict.Causes[0].Kind]
	if !known {
		return messageCritical
	}

	return message
}
