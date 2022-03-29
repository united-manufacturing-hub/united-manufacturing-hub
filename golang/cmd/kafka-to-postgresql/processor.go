package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"regexp"
)

var rp = regexp.MustCompile(`ia\.([\w]*)\.([\w]*)\.([\w]*)\.([\w]*)`)
var countChannel = make(chan *kafka.Message, 100000)

type ParsedMessage struct {
	AssetId     string
	Location    string
	CustomerId  string
	PayloadType string
	Payload     []byte
}

func ParseMessage(msg *kafka.Message, qPid int) (bool, ParsedMessage) {

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

func queueProcessor(qPid int) {
	zap.S().Debugf("[%d] Starting queue processor", qPid)
	for !ShuttingDown {
		var msg *kafka.Message
		msg = <-processorChannel
		parsed, parsedMessage := ParseMessage(msg, qPid)
		if !parsed {
			continue
		}

		var err error
		var putback bool

		switch parsedMessage.PayloadType {
		case Prefix.ProcessValueFloat64:
			err, putback = ProcessValueFloat64{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ProcessValue:
			err, putback = ProcessValue{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ProcessValueString:
			err, putback = ProcessValueString{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.Count:
			countChannel <- msg
			continue
		case Prefix.Recommendation:
			zap.S().Errorf("[%d] Recommendation message not implemented", qPid)
			//err, putback = Recommendation{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.State:
			err, putback = State{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.UniqueProduct:
			err, putback = UniqueProduct{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ScrapCount:
			err, putback = ScrapCount{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.AddShift:
			err, putback = AddShift{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ScrapUniqueProduct:
			err, putback = ScrapUniqueProduct{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.AddProduct:
			err, putback = AddProduct{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.AddOrder:
			err, putback = AddOrder{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.StartOrder:
			err, putback = StartOrder{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.EndOrder:
			err, putback = EndOrder{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.AddMaintenanceActivity:
			zap.S().Errorf("[%d] AddMaintenanceActivity message not implemented", qPid)
			//err, putback = AddMaintenanceActivity{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ProductTag:
			err, putback = ProductTag{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ProductTagString:
			err, putback = ProductTagString{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.AddParentToChild:
			err, putback = AddParentToChild{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ModifyState:
			zap.S().Errorf("[%d] ModifyState message not implemented", qPid)
			//err, putback = ModifyState{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.ModifyProducesPieces:
			zap.S().Errorf("[%d] ModifyProducesPieces message not implemented", qPid)
			//err, putback = ModifyProducesPieces{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.DeleteShiftById:
			zap.S().Errorf("[%d] DeleteShiftById message not implemented", qPid)
			//err, putback = DeleteShiftById{}.ProcessMessages(parsedMessage, qPid)
			break
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			zap.S().Errorf("[%d] DeleteShiftByAssetIdAndBeginTimestamp message not implemented", qPid)
			//err, putback = DeleteShiftByAssetIdAndBeginTimestamp{}.ProcessMessages(parsedMessage, qPid)
			break
		}

		if err != nil {
			errStr := err.Error()
			switch GetPostgresErrorRecoveryOptions(err) {
			case DatabaseDown:
				if putback {
					zap.S().Errorf("[%d][DatabaseDown] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Putting back to queue", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
					putBackChannel <- PutBackChan{msg: msg, reason: "DatabaseDown", errorString: &errStr}
				} else {
					zap.S().Errorf("[%d][DatabaseDown] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Discarding message", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
				}
				break
			case DiscardValue:
				zap.S().Errorf("[%d][DiscardValue] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Discarding message", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
				break
			case Other:
				if putback {
					zap.S().Errorf("[%d][Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Putting back to queue", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
					putBackChannel <- PutBackChan{msg: msg, reason: "Other", errorString: &errStr}
				} else {
					zap.S().Errorf("[%d][Other] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v. Discarding message", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload, err)
				}
				break
			}
		} else {
			if putback {
				zap.S().Errorf("[%d][No-Error Putback] Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Putting back to queue", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload)
				putBackChannel <- PutBackChan{msg: msg, reason: "Other", errorString: nil}
			} else {
				zap.S().Debugf("[%d][Success] Successfully executed Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload)
			}
		}
	}
	zap.S().Debugf("[%d] Processor shutting down", qPid)
}
