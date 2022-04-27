package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// startHighIntegrityQueueProcessor starts the kafka processor for the high integrity queue
func startHighIntegrityQueueProcessor() {
	if !HighIntegrityEnabled {
		return
	}
	zap.S().Debugf("[HI]Starting queue processor")
	for !ShuttingDown {
		var msg *kafka.Message
		// Get next message from HI kafka consumer
		msg = <-highIntegrityProcessorChannel
		if msg == nil {
			continue
		}
		parsed, parsedMessage := internal.ParseMessage(msg)
		if !parsed {
			continue
		}

		var err error
		var putback bool

		// Switch based on topic
		switch parsedMessage.PayloadType {
		case Prefix.Count:
			err, putback = Count{}.ProcessMessages(parsedMessage)
			break
		case Prefix.Recommendation:
			zap.S().Errorf("[HI]Recommendation message not implemented")
			//err, putback = Recommendation{}.ProcessMessages(parsedMessage)
			break
		case Prefix.State:
			err, putback = State{}.ProcessMessages(parsedMessage)
			break
		case Prefix.UniqueProduct:
			err, putback = UniqueProduct{}.ProcessMessages(parsedMessage)
			break
		case Prefix.ScrapCount:
			err, putback = ScrapCount{}.ProcessMessages(parsedMessage)
			break
		case Prefix.AddShift:
			err, putback = AddShift{}.ProcessMessages(parsedMessage)
			break
		case Prefix.ScrapUniqueProduct:
			err, putback = ScrapUniqueProduct{}.ProcessMessages(parsedMessage)
			break
		case Prefix.AddProduct:
			err, putback = AddProduct{}.ProcessMessages(parsedMessage)
			break
		case Prefix.AddOrder:
			err, putback = AddOrder{}.ProcessMessages(parsedMessage)
			break
		case Prefix.StartOrder:
			err, putback = StartOrder{}.ProcessMessages(parsedMessage)
			break
		case Prefix.EndOrder:
			err, putback = EndOrder{}.ProcessMessages(parsedMessage)
			break
		case Prefix.AddMaintenanceActivity:
			zap.S().Errorf("[HI]AddMaintenanceActivity message not implemented")
			//err, putback = AddMaintenanceActivity{}.ProcessMessages(parsedMessage)
			break
		case Prefix.ProductTag:
			err, putback = ProductTag{}.ProcessMessages(parsedMessage)
			break
		case Prefix.ProductTagString:
			err, putback = ProductTagString{}.ProcessMessages(parsedMessage)
			break
		case Prefix.AddParentToChild:
			err, putback = AddParentToChild{}.ProcessMessages(parsedMessage)
			break
		case Prefix.ModifyState:
			zap.S().Errorf("[HI]ModifyState message not implemented")
			//err, putback = ModifyState{}.ProcessMessages(parsedMessage)
			break
		case Prefix.ModifyProducesPieces:
			zap.S().Errorf("[HI]ModifyProducesPieces message not implemented")
			//err, putback = ModifyProducesPieces{}.ProcessMessages(parsedMessage)
			break
		case Prefix.DeleteShiftById:
			zap.S().Errorf("[HI]DeleteShiftById message not implemented")
			//err, putback = DeleteShiftById{}.ProcessMessages(parsedMessage)
			break
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			zap.S().Errorf("[HI]DeleteShiftByAssetIdAndBeginTimestamp message not implemented")
			//err, putback = DeleteShiftByAssetIdAndBeginTimestamp{}.ProcessMessages(parsedMessage)
			break
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
					highIntegrityPutBackChannel <- internal.PutBackChanMsg{Msg: msg, Reason: "DatabaseDown", ErrorString: &errStr}
				} else {
					zap.S().Errorf("[HI][DatabaseDown] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
					highIntegrityCommitChannel <- msg
				}
				break
			case DiscardValue:
				zap.S().Errorf("[HI][DiscardValue] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
				highIntegrityCommitChannel <- msg
				break
			case Other:
				if putback {

					zap.S().Debugf("[HI][Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)

					highIntegrityPutBackChannel <- internal.PutBackChanMsg{Msg: msg, Reason: "Other", ErrorString: &errStr}
				} else {
					zap.S().Errorf("[HI][Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
					highIntegrityCommitChannel <- msg
				}
				break
			}
		} else {
			if putback {
				payloadStr := string(parsedMessage.Payload)

				zap.S().Debugf("[HI][No-Error Putback] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr)
				highIntegrityPutBackChannel <- internal.PutBackChanMsg{Msg: msg, Reason: "Other"}

			} else {
				highIntegrityCommitChannel <- msg
			}
		}
	}
	zap.S().Debugf("[HI]Processor shutting down")
}
