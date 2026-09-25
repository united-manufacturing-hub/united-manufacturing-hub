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
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/channelusage"
	pullsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	pushsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	transportsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	transportstate "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// transportMaxAge bounds how old the transport worker's observation may be
// before it is reported degraded.
const transportMaxAge = 10 * time.Second

var (
	transportRef = dynamicchildren.Ref{WorkerType: "transport", Name: "transport"}
	pushRef      = dynamicchildren.Ref{WorkerType: "push", Name: "push"}
	pullRef      = dynamicchildren.Ref{WorkerType: "pull", Name: "pull"}
)

var (
	transportRunning    = (&transportstate.RunningState{}).String()
	transportDegraded   = (&transportstate.DegradedState{}).String()
	transportStarting   = (&transportstate.StartingState{}).String()
	transportAuthFailed = (&transportstate.AuthFailedState{}).String()
)

// CommunicatorFromFSMv2 reads the fsmv2 transport worker and its push and pull
// children and maps them to models.Communicator. It returns nil when the fsmv2
// client is unavailable.
func CommunicatorFromFSMv2(ctx context.Context, log *zap.SugaredLogger, subscriberCount int) *models.Communicator {
	client := fsmv2client.GetClient()
	if client == nil {
		return nil
	}

	var result *models.Communicator

	transport, err := fsmv2client.Get[transportsnapshot.TransportStatus](ctx, client, transportRef)

	switch {
	case err != nil:
		if !errors.Is(err, fsmv2client.ErrNotObserved) {
			log.Warnw("communicator status: failed to read transport observed state", "error", err)
		}

		result = &models.Communicator{Health: communicatorHealthOf(models.Neutral, "Communicator status unknown")}
	case time.Since(transport.CollectedAt) > transportMaxAge:
		result = &models.Communicator{Health: communicatorHealthOf(models.Degraded, "Communicator status is stale")}
	default:
		result = CommunicatorFromObservations(
			transport,
			observationOrZero[pushsnapshot.PushStatus](ctx, client, pushRef),
			observationOrZero[pullsnapshot.PullStatus](ctx, client, pullRef),
		)
	}

	result.SubscriberCount = subscriberCount

	return result
}

// CommunicatorFromObservations maps the transport worker's observation and its
// children's to models.Communicator. A child with a zero observation is
// reported as not yet observed.
func CommunicatorFromObservations(
	transport fsmv2.Observation[transportsnapshot.TransportStatus],
	push fsmv2.Observation[pushsnapshot.PushStatus],
	pull fsmv2.Observation[pullsnapshot.PullStatus],
) *models.Communicator {
	status := transport.Status

	return &models.Communicator{
		Health:                     communicatorHealth(transport.State, status),
		State:                      transport.State,
		ConsecutiveErrors:          status.ConsecutiveErrors,
		LastErrorType:              errorTypeName(status.ConsecutiveErrors, status.LastErrorType),
		OutboundChannelFillPercent: status.OutboundQueue.FillPercent,
		OutboundChannelPeakPercent: status.OutboundQueue.PeakPercent,
		Push:                       pushChannel(push),
		Pull:                       pullChannel(pull),
	}
}

// observationOrZero reads ref's observation, or returns a zero observation when
// the read fails.
func observationOrZero[TStatus any](ctx context.Context, client *fsmv2client.FSMv2Client, ref dynamicchildren.Ref) fsmv2.Observation[TStatus] {
	observed, err := fsmv2client.Get[TStatus](ctx, client, ref)
	if err != nil {
		return fsmv2.Observation[TStatus]{}
	}

	return observed
}

func pushChannel(observed fsmv2.Observation[pushsnapshot.PushStatus]) *models.CommunicatorChannel {
	if observed.CollectedAt.IsZero() {
		return nil
	}

	status := observed.Status
	metrics := observed.Metrics.Worker

	return &models.CommunicatorChannel{
		State:             observed.State,
		LastErrorType:     errorTypeName(status.ConsecutiveErrors, status.LastErrorType),
		Messages:          metrics.Counters[string(deps.CounterMessagesPushed)],
		MessagesDropped:   metrics.Counters[string(deps.CounterMessagesDropped)],
		PendingMessages:   status.PendingMessageCount,
		ConsecutiveErrors: status.ConsecutiveErrors,
		LastStatusCode:    status.LastStatusCode,
		LastLatencyMs:     metrics.Gauges[string(deps.GaugeLastPushLatencyMs)],
	}
}

func pullChannel(observed fsmv2.Observation[pullsnapshot.PullStatus]) *models.CommunicatorChannel {
	if observed.CollectedAt.IsZero() {
		return nil
	}

	status := observed.Status
	metrics := observed.Metrics.Worker

	return &models.CommunicatorChannel{
		State:             observed.State,
		LastErrorType:     errorTypeName(status.ConsecutiveErrors, status.LastErrorType),
		Messages:          metrics.Counters[string(deps.CounterMessagesPulled)],
		MessagesDropped:   metrics.Counters[string(deps.CounterMessagesDropped)],
		PendingMessages:   status.PendingMessageCount,
		ConsecutiveErrors: status.ConsecutiveErrors,
		LastStatusCode:    status.LastStatusCode,
		LastLatencyMs:     metrics.Gauges[string(deps.GaugeLastPullLatencyMs)],
		Backpressured:     status.IsBackpressured,
	}
}

// errorTypeName names the last error while errors are ongoing, and is empty
// otherwise.
func errorTypeName(consecutiveErrors int, errorType types.ErrorType) string {
	if consecutiveErrors == 0 {
		return ""
	}

	return errorType.String()
}

func communicatorHealth(state string, status transportsnapshot.TransportStatus) *models.Health {
	queue := status.OutboundQueue
	usage := fmt.Sprintf(
		"Outbound queue %.0f%% full, peak %.0f%% in the last %.0fs",
		queue.FillPercent, queue.PeakPercent, channelusage.Window.Seconds(),
	)

	switch {
	case queue.Degraded:
		return communicatorHealthOf(models.Degraded, usage+". Status updates and action replies may be lost")
	case state == transportAuthFailed:
		return communicatorHealthOf(models.Degraded, "Authentication with the Management Console failed")
	case state == transportDegraded:
		message := "Connection to the Management Console is degraded"
		if name := errorTypeName(status.ConsecutiveErrors, status.LastErrorType); name != "" {
			message += fmt.Sprintf(": %d consecutive %s errors", status.ConsecutiveErrors, name)
		}

		return communicatorHealthOf(models.Degraded, message)
	case state == transportStarting:
		return communicatorHealthOf(models.Neutral, "Connecting to the Management Console")
	case state != transportRunning:
		return communicatorHealthOf(models.Neutral, "Communicator not running")
	case !queue.Measured:
		return communicatorHealthOf(models.Neutral, "Measuring outbound queue usage")
	default:
		return communicatorHealthOf(models.Active, usage)
	}
}

func communicatorHealthOf(category models.HealthCategory, message string) *models.Health {
	return &models.Health{
		Message:       message,
		ObservedState: category.String(),
		DesiredState:  models.Active.String(),
		Category:      category,
	}
}
