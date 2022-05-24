package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// startHighIntegrityQueueProcessor starts the kafka processor for the high integrity queue
func startHighIntegrityQueueProcessor() {

	zap.S().Debugf("[HI]Starting queue processor")
	for !ShuttingDown {
		// Get next message from HI kafka consumer
		msg := <-highIntegrityProcessorChannel
		if msg == nil {
			continue
		}
		parsed, parsedMessage := internal.ParseMessage(msg)
		if !parsed {
			continue
		}

		var err error
		var putback bool
		var forcePBTopic bool

		// Switch based on topic
		switch parsedMessage.PayloadType {
		case Prefix.Count:
			putback, err, forcePBTopic = Count{}.ProcessMessages(parsedMessage)
		case Prefix.Recommendation:
			zap.S().Errorf("[HI]Recommendation is unstable")
			putback, err, forcePBTopic = Recommendation{}.ProcessMessages(parsedMessage)
		case Prefix.State:
			putback, err, forcePBTopic = State{}.ProcessMessages(parsedMessage)
		case Prefix.UniqueProduct:
			putback, err, forcePBTopic = UniqueProduct{}.ProcessMessages(parsedMessage)
		case Prefix.ScrapCount:
			putback, err, forcePBTopic = ScrapCount{}.ProcessMessages(parsedMessage)
		case Prefix.AddShift:
			putback, err, forcePBTopic = AddShift{}.ProcessMessages(parsedMessage)
		case Prefix.ScrapUniqueProduct:
			putback, err, forcePBTopic = ScrapUniqueProduct{}.ProcessMessages(parsedMessage)
		case Prefix.AddProduct:
			putback, err, forcePBTopic = AddProduct{}.ProcessMessages(parsedMessage)
		case Prefix.AddOrder:
			putback, err, forcePBTopic = AddOrder{}.ProcessMessages(parsedMessage)
		case Prefix.StartOrder:
			putback, err, forcePBTopic = StartOrder{}.ProcessMessages(parsedMessage)
		case Prefix.EndOrder:
			putback, err, forcePBTopic = EndOrder{}.ProcessMessages(parsedMessage)
		case Prefix.AddMaintenanceActivity:
			zap.S().Errorf("[HI]AddMaintenanceActivity is unstable")
			putback, err, forcePBTopic = AddMaintenanceActivity{}.ProcessMessages(parsedMessage)
		case Prefix.ProductTag:
			putback, err, forcePBTopic = ProductTag{}.ProcessMessages(parsedMessage)
		case Prefix.ProductTagString:
			putback, err, forcePBTopic = ProductTagString{}.ProcessMessages(parsedMessage)
		case Prefix.AddParentToChild:
			putback, err, forcePBTopic = AddParentToChild{}.ProcessMessages(parsedMessage)
		case Prefix.ModifyState:
			zap.S().Errorf("[HI]ModifyState is unstable")
			putback, err, forcePBTopic = ModifyState{}.ProcessMessages(parsedMessage)
		case Prefix.ModifyProducesPieces:
			zap.S().Errorf("[HI]ModifyProducesPieces is unstable")
			putback, err, forcePBTopic = ModifyProducesPieces{}.ProcessMessages(parsedMessage)
		case Prefix.DeleteShiftById:
			zap.S().Errorf("[HI]DeleteShiftById is unstable")
			putback, err, forcePBTopic = DeleteShiftById{}.ProcessMessages(parsedMessage)
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			zap.S().Errorf("[HI]DeleteShiftByAssetIdAndBeginTimestamp is unstable")
			putback, err, forcePBTopic = DeleteShiftByAssetIdAndBeginTimestamp{}.ProcessMessages(parsedMessage)

		default:
			zap.S().Warnf("[HI] Prefix not allowed: %s, putting back", parsedMessage.PayloadType)
			putback = true
		}

		if err != nil {
			payloadStr := string(parsedMessage.Payload)
			errStr := err.Error()
			switch GetPostgresErrorRecoveryOptions(err) {
			case DatabaseDown:
				if putback {
					zap.S().Debugf("[HI][DatabaseDown] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
					highIntegrityPutBackChannel <- internal.PutBackChanMsg{Msg: msg, Reason: "DatabaseDown", ErrorString: &errStr, ForcePutbackTopic: forcePBTopic}
				} else {
					zap.S().Errorf("[HI][DatabaseDown] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
					highIntegrityCommitChannel <- msg
				}

			case DiscardValue:
				zap.S().Errorf("[HI][DiscardValue] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
				highIntegrityCommitChannel <- msg
			case Other:
				if putback {

					zap.S().Debugf("[HI][Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)

					highIntegrityPutBackChannel <- internal.PutBackChanMsg{Msg: msg, Reason: "Other (Error)", ErrorString: &errStr, ForcePutbackTopic: forcePBTopic}
				} else {
					zap.S().Errorf("[HI][Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
					highIntegrityCommitChannel <- msg
				}
			}
		} else {
			if putback {
				payloadStr := string(parsedMessage.Payload)

				zap.S().Debugf("[HI][No-Error Putback] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s, topic: %s, PayloadType: %s. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, *msg.TopicPartition.Topic, parsedMessage.PayloadType)
				highIntegrityPutBackChannel <- internal.PutBackChanMsg{Msg: msg, Reason: "Other (No-Error)", ForcePutbackTopic: forcePBTopic}
			} else {
				highIntegrityCommitChannel <- msg
			}
		}
		if forcePBTopic {
			highIntegrityCommitChannel <- msg
		}
	}

	zap.S().Debugf("[HI]Processor shutting down")
}
