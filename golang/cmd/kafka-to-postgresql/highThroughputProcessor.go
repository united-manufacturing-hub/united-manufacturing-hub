// Copyright 2023 UMH Systems GmbH
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

package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// startHighThroughputQueueProcessor starts the kafka processor for the high integrity queue
func startHighThroughputQueueProcessor() {

	zap.S().Debugf("[HT]Starting queue processor")
	for !ShuttingDown {
		msg := <-highThroughputProcessorChannel
		if msg == nil {
			continue
		}
		parsed, parsedMessage := internal.ParseMessage(msg)
		if !parsed {
			continue
		}

		var putback bool

		switch parsedMessage.PayloadType {
		case Prefix.ProcessValueFloat64:
			processValueChannel <- msg

		case Prefix.ProcessValue:
			processValueChannel <- msg

		case Prefix.ProcessValueString:
			processValueStringChannel <- msg

		default:
			zap.S().Warnf("[HT] Prefix not allowed: %s, putting back", parsedMessage.PayloadType)
			putback = true
		}

		if putback {
			payloadStr := string(parsedMessage.Payload)

			zap.S().Debugf("[HT][No-Error Putback] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr)
			highThroughputPutBackChannel <- internal.PutBackChanMsg{Msg: msg, Reason: "Other"}

		}
	}
	zap.S().Debugf("[HT]Processor shutting down")
}
