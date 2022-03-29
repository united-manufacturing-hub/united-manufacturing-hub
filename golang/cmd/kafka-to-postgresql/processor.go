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
			break
		case Prefix.ProcessValue:
			break
		case Prefix.ProcessValueString:
			break
		case Prefix.Count:
			countChannel <- msg
			continue
		case Prefix.Recommendation:
			break
		case Prefix.State:
			break
		case Prefix.UniqueProduct:
			break
		case Prefix.ScrapCount:
			break
		case Prefix.AddShift:
			break
		case Prefix.UniqueProductScrap:
			break
		case Prefix.AddProduct:
			break
		case Prefix.AddOrder:
			break
		case Prefix.StartOrder:
			break
		case Prefix.EndOrder:
			break
		case Prefix.AddMaintenanceActivity:
			break
		case Prefix.ProductTag:
			break
		case Prefix.ProductTagString:
			break
		case Prefix.AddParentToChild:
			break
		case Prefix.ModifyState:
			break
		case Prefix.ModifyProducesPieces:
			break
		case Prefix.DeleteShiftById:
			break
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
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
			zap.S().Debugf("[%d][Success] Successfully executed Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %s", qPid, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, parsedMessage.Payload)
		}
	}
	zap.S().Debugf("[%d] Processor shutting down", qPid)
}
