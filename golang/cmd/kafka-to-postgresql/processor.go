package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql/processors"
	"go.uber.org/zap"
	"regexp"
)

var rp = regexp.MustCompile(`ia/([\w]*)/([\w]*)/([\w]*)/([\w]*)`)

func queueProcessor() {
	for !ShuttingDown {
		var msg *kafka.Message
		msg = <-processorChannel

		res := rp.FindStringSubmatch(*msg.TopicPartition.Topic)
		if res == nil {
			continue
		}

		customerID := res[1]
		location := res[2]
		assetID := res[3]
		payloadType := res[4]
		payload := msg.Value

		var err error
		switch payloadType {
		case Prefix.ProcessValueFloat64:
			break
		case Prefix.ProcessValue:
			break
		case Prefix.ProcessValueString:
			break
		case Prefix.Count:
			break
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
			err = processors.AddOrder{}.ProcessMessage(customerID, location, assetID, payload)
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
			zap.S().Errorf("Failed to execute Kafka message. CustomerID: %s, Location: %s, AssetId: %s, payload: %v. Error: %v", customerID, location, assetID, payload, err)
			PutBack(msg)
		} else {
			CommitMessage(msg)
		}
	}
}
