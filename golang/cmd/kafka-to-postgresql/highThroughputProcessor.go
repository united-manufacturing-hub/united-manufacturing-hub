package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
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
		var msg *kafka.Message
		msg = <-highThroughputProcessorChannel
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
			break
		case Prefix.ProcessValue:
			processValueChannel <- msg
			break
		case Prefix.ProcessValueString:
			processValueStringChannel <- msg
			break
		default:
			zap.S().Warnf("[HT] Prefix not allowed: %s, putting back", parsedMessage.PayloadType)
			putback = true
		}

		if putback {
			zap.S().Errorf("[HT][No-Error KafkaPutback] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload)
			highThroughputPutBackChannel <- PutBackChanMsg{msg: msg, reason: "Other", errorString: nil}
		}
	}
	zap.S().Debugf("[HT]Processor shutting down")
}
