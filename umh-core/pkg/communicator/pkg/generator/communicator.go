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

package generator

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/channelusage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// CommunicatorFromMonitor converts the outbound channel verdict into the wire
// shape. Health is Neutral until the monitor has a verdict.
func CommunicatorFromMonitor(monitor *channelusage.Monitor, subscriberCount int) *models.Communicator {
	communicator := &models.Communicator{
		Health:          outboundChannelUnknown(),
		SubscriberCount: subscriberCount,
	}

	verdict, ok := monitor.Verdict()
	if !ok {
		return communicator
	}

	communicator.Health = outboundChannelHealth(verdict)
	communicator.OutboundChannelFillPercent = verdict.P95FillPercent
	communicator.OutboundChannelPeakPercent = verdict.PeakFillPercent

	return communicator
}

func outboundChannelUnknown() *models.Health {
	return &models.Health{
		Message:       "Measuring outbound queue usage",
		ObservedState: models.Neutral.String(),
		DesiredState:  models.Active.String(),
		Category:      models.Neutral,
	}
}

func outboundChannelHealth(verdict channelusage.Verdict) *models.Health {
	usage := fmt.Sprintf(
		"Outbound queue %.0f%% full, peak %.0f%% in the last %.0fs",
		verdict.P95FillPercent, verdict.PeakFillPercent, channelusage.Window.Seconds(),
	)

	if verdict.Degraded {
		return &models.Health{
			Message:       usage + ". Status updates and action replies may be lost",
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}
	}

	return &models.Health{
		Message:       usage,
		ObservedState: models.Active.String(),
		DesiredState:  models.Active.String(),
		Category:      models.Active,
	}
}
