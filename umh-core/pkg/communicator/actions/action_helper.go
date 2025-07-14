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

package actions

import (
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// ConsumeOutboundMessages processes messages from the outbound channel
// This method is used for testing purposes to consume messages that would normally be sent to the user
func ConsumeOutboundMessages(outboundChannel chan *models.UMHMessage, messages *[]*models.UMHMessage, logMessages bool) {
	for msg := range outboundChannel {
		*messages = append(*messages, msg)
		decodedMessage, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
		if err != nil {
			zap.S().Error("error decoding message", zap.Error(err))
			continue
		}
		if logMessages {
			zap.S().Info("received message", decodedMessage.Payload)
		}

	}
}

// SendLimitedLogs sends a maximum of 10 logs to the user and a message about remaining logs.
// Returns the updated lastLogs array that includes all logs, even those not sent.
func SendLimitedLogs(
	logs []process_shared.LogEntry,
	lastLogs []process_shared.LogEntry,
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	actionType models.ActionType,
	remainingSeconds int) []process_shared.LogEntry {

	if len(logs) <= len(lastLogs) {
		return lastLogs
	}

	maxLogsToSend := 10
	logsToSend := logs[len(lastLogs):]
	remainingLogs := len(logsToSend) - maxLogsToSend

	// Send at most maxLogsToSend logs
	end := min(len(logsToSend), maxLogsToSend)

	for _, log := range logsToSend[:end] {
		stateMessage := RemainingPrefixSec(remainingSeconds) + "received log line: " + log.Content
		SendActionReply(instanceUUID, userEmail, actionUUID, models.ActionExecuting,
			stateMessage,
			outboundChannel, actionType)
	}

	// Send message about remaining logs if any
	if remainingLogs > 0 {
		stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("%d remaining logs not displayed", remainingLogs)
		SendActionReply(instanceUUID, userEmail, actionUUID, models.ActionExecuting,
			stateMessage,
			outboundChannel, actionType)
	}

	// Return updated lastLogs to include all logs we've seen, even if not all were sent
	return logs
}

// RemainingPrefixSec formats d (assumed ≤20 s) as "[left: NN s] ".
func RemainingPrefixSec(dSeconds int) string {
	return fmt.Sprintf("[left: %02d s] ", dSeconds) // fixed 15-rune prefix
}

// High-level label for one-off (non-polling) messages.
//
//	action = "deploy", "edit" …
//	name   = human name of the component
//
// → "deploy(foo): "
func Label(action, name string) string {
	return fmt.Sprintf("%s(%s): ", action, name)
}
