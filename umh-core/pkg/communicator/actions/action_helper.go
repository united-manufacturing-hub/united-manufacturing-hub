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
