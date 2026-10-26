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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// communicatorFromStatus copies the connection's status and umh-core's
// subscriber count into the status message's connection part. It returns nil
// for a nil status.
func communicatorFromStatus(st *managementconsole.Status, subscriberCount int) *models.Communicator {
	if st == nil {
		return nil
	}

	return &models.Communicator{
		Health:                     communicatorHealthOf(healthCategoryOf(st.Health), st.HealthMessage),
		State:                      st.State,
		SubscriberCount:            subscriberCount,
		ConsecutiveErrors:          st.ConsecutiveErrors,
		LastErrorType:              st.LastErrorType,
		OutboundChannelFillPercent: st.OutboundFillPercent,
		OutboundChannelPeakPercent: st.OutboundPeakPercent,
		Push:                       channelFromStatus(st.Push),
		Pull:                       channelFromStatus(st.Pull),
	}
}

// communicatorHealthOf builds the connection part's health. The connection
// should always be active, so the desired state is Active.
func communicatorHealthOf(category models.HealthCategory, message string) *models.Health {
	return &models.Health{Message: message, ObservedState: category.String(), DesiredState: models.Active.String(), Category: category}
}

// healthCategoryOf maps the connection's health to the status message's
// category. An unknown value maps to Neutral.
func healthCategoryOf(h managementconsole.HealthCategory) models.HealthCategory {
	switch h {
	case managementconsole.Active:
		return models.Active
	case managementconsole.Degraded:
		return models.Degraded
	default:
		return models.Neutral
	}
}

// channelFromStatus copies one direction of the connection. It returns nil for
// a nil status.
func channelFromStatus(c *managementconsole.ChannelStatus) *models.CommunicatorChannel {
	if c == nil {
		return nil
	}

	return &models.CommunicatorChannel{
		State:             c.State,
		LastErrorType:     c.LastErrorType,
		Messages:          c.Messages,
		MessagesDropped:   c.MessagesDropped,
		PendingMessages:   c.PendingMessages,
		ConsecutiveErrors: c.ConsecutiveErrors,
		LastStatusCode:    c.LastStatusCode,
		LastLatencyMs:     c.LastLatencyMs,
		Backpressured:     c.Backpressured,
	}
}
