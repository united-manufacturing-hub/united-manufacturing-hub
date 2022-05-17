package main

import (
	kafka2 "github.com/united-manufacturing-hub/umh-lib/v2/kafka"
	"go.uber.org/zap"
)

// startHighIntegrityQueueProcessor starts the kafka processor for the high integrity queue
func startHighIntegrityQueueProcessor() {
	if !HighIntegrityEnabled {
		return
	}
	zap.S().Debugf("[HI]Starting queue processor")
	for !ShuttingDown {
		// Get next message from HI kafka consumer
		msg := <-highIntegrityProcessorChannel
		if msg == nil {
			continue
		}
		parsed, parsedMessage := kafka2.ParseMessage(msg)
		if !parsed {
			continue
		}

		var err error
		var putback bool

		// Switch based on topic
		switch parsedMessage.PayloadType {
		case Prefix.Count:
			putback, err = Count{}.ProcessMessages(parsedMessage)
		case Prefix.Recommendation:
			zap.S().Errorf("[HI]Recommendation message not implemented")
			//putback, err = Recommendation{}.ProcessMessages(parsedMessage)
		case Prefix.State:
			putback, err = State{}.ProcessMessages(parsedMessage)
		case Prefix.UniqueProduct:
			putback, err = UniqueProduct{}.ProcessMessages(parsedMessage)
		case Prefix.ScrapCount:
			putback, err = ScrapCount{}.ProcessMessages(parsedMessage)
		case Prefix.AddShift:
			putback, err = AddShift{}.ProcessMessages(parsedMessage)
		case Prefix.ScrapUniqueProduct:
			putback, err = ScrapUniqueProduct{}.ProcessMessages(parsedMessage)
		case Prefix.AddProduct:
			putback, err = AddProduct{}.ProcessMessages(parsedMessage)
		case Prefix.AddOrder:
			putback, err = AddOrder{}.ProcessMessages(parsedMessage)
		case Prefix.StartOrder:
			putback, err = StartOrder{}.ProcessMessages(parsedMessage)
		case Prefix.EndOrder:
			putback, err = EndOrder{}.ProcessMessages(parsedMessage)
		case Prefix.AddMaintenanceActivity:
			zap.S().Errorf("[HI]AddMaintenanceActivity message not implemented")
			//putback, err = AddMaintenanceActivity{}.ProcessMessages(parsedMessage)
		case Prefix.ProductTag:
			putback, err = ProductTag{}.ProcessMessages(parsedMessage)
		case Prefix.ProductTagString:
			putback, err = ProductTagString{}.ProcessMessages(parsedMessage)
		case Prefix.AddParentToChild:
			putback, err = AddParentToChild{}.ProcessMessages(parsedMessage)
		case Prefix.ModifyState:
			zap.S().Errorf("[HI]ModifyState message not implemented")
			//putback, err = ModifyState{}.ProcessMessages(parsedMessage)
		case Prefix.ModifyProducesPieces:
			zap.S().Errorf("[HI]ModifyProducesPieces message not implemented")
			//putback, err = ModifyProducesPieces{}.ProcessMessages(parsedMessage)
		case Prefix.DeleteShiftById:
			zap.S().Errorf("[HI]DeleteShiftById message not implemented")
			//putback, err = DeleteShiftById{}.ProcessMessages(parsedMessage)
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			zap.S().Errorf("[HI]DeleteShiftByAssetIdAndBeginTimestamp message not implemented")
			//putback, err = DeleteShiftByAssetIdAndBeginTimestamp{}.ProcessMessages(parsedMessage)

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
					highIntegrityPutBackChannel <- kafka2.PutBackChanMsg{Msg: msg, Reason: "DatabaseDown", ErrorString: &errStr}
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

					highIntegrityPutBackChannel <- kafka2.PutBackChanMsg{Msg: msg, Reason: "Other (Error)", ErrorString: &errStr}
				} else {
					zap.S().Errorf("[HI][Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr, err)
					highIntegrityCommitChannel <- msg
				}
			}
		} else {
			if putback {
				payloadStr := string(parsedMessage.Payload)

				zap.S().Debugf("[HI][No-Error Putback] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, payloadStr)
				highIntegrityPutBackChannel <- kafka2.PutBackChanMsg{Msg: msg, Reason: "Other (No-Error)"}

			} else {
				highIntegrityCommitChannel <- msg
			}
		}
	}
	zap.S().Debugf("[HI]Processor shutting down")
}
