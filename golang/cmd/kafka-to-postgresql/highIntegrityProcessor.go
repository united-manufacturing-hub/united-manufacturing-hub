package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"regexp"
)

var rp = regexp.MustCompile(`ia\.([\w]*)\.([\w]*)\.([\w]*)\.([\w]*)`)

type ParsedMessage struct {
	AssetId     string
	Location    string
	CustomerId  string
	PayloadType string
	Payload     []byte
}

func ParseMessage(msg *kafka.Message) (bool, ParsedMessage) {

	valid, found, message := GetCacheParsedMessage(msg)
	if !valid {
		return false, ParsedMessage{}
	}
	if found {
		return true, message
	}

	valid, m := PutCacheParsedMessage(msg)
	if !valid {
		return false, ParsedMessage{}
	}

	return true, m

}

func startHighIntegrityQueueProcessor() {
	if !HighIntegrityEnabled {
		return
	}
	zap.S().Debugf("Starting queue processor")
	for !ShuttingDown {
		var msg *kafka.Message
		msg = <-highIntegrityProcessorChannel
		parsed, parsedMessage := ParseMessage(msg)
		if !parsed {
			continue
		}

		var err error
		var putback bool

		switch parsedMessage.PayloadType {
		case Prefix.ProcessValueFloat64:
			zap.S().Errorf("ProcessValueFloat64 not allowed for high integrity")
			break
		case Prefix.ProcessValue:
			zap.S().Errorf("ProcessValue not allowed for high integrity")
			break
		case Prefix.ProcessValueString:
			zap.S().Errorf("ProcessValueString not allowed for high integrity")
			break
		case Prefix.Count:
			err, putback = Count{}.ProcessMessages(parsedMessage)
			break
		case Prefix.Recommendation:
			zap.S().Errorf("Recommendation message not implemented")
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
			zap.S().Errorf("AddMaintenanceActivity message not implemented")
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
			zap.S().Errorf("ModifyState message not implemented")
			//err, putback = ModifyState{}.ProcessMessages(parsedMessage)
			break
		case Prefix.ModifyProducesPieces:
			zap.S().Errorf("ModifyProducesPieces message not implemented")
			//err, putback = ModifyProducesPieces{}.ProcessMessages(parsedMessage)
			break
		case Prefix.DeleteShiftById:
			zap.S().Errorf("DeleteShiftById message not implemented")
			//err, putback = DeleteShiftById{}.ProcessMessages(parsedMessage)
			break
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			zap.S().Errorf("DeleteShiftByAssetIdAndBeginTimestamp message not implemented")
			//err, putback = DeleteShiftByAssetIdAndBeginTimestamp{}.ProcessMessages(parsedMessage)
			break
		}

		if err != nil {
			errStr := err.Error()
			switch GetPostgresErrorRecoveryOptions(err) {
			case DatabaseDown:
				if putback {
					zap.S().Errorf("[DatabaseDown] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
					highIntegrityPutBackChannel <- PutBackChan{msg: msg, reason: "DatabaseDown", errorString: &errStr}
				} else {
					zap.S().Errorf("[DatabaseDown] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
					highIntegrityCommitChannel <- msg
				}
				break
			case DiscardValue:
				//zap.S().Errorf("[DiscardValue] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
				highIntegrityCommitChannel <- msg
				break
			case Other:
				if putback {
					zap.S().Errorf("[Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
					highIntegrityPutBackChannel <- PutBackChan{msg: msg, reason: "Other", errorString: &errStr}
				} else {
					zap.S().Errorf("[Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Discarding message", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
					highIntegrityCommitChannel <- msg
				}
				break
			}
		} else {
			if putback {
				zap.S().Errorf("[No-Error Putback] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Putting back to queue", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload)
				highIntegrityPutBackChannel <- PutBackChan{msg: msg, reason: "Other", errorString: nil}
			} else {
				highIntegrityCommitChannel <- msg
			}
		}
	}
	zap.S().Debugf("Processor shutting down")
}
