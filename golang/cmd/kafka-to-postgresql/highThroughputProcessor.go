package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// startHighThroughputQueueProcessor starts the kafka processor for the high integrity queue
func startHighThroughputQueueProcessor() {
	if !HighThroughputEnabled {
		return
	}
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
