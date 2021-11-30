package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"strings"
	"time"
)

func setupKafka(boostrapServer string) (consumer *kafka.Consumer, producer *kafka.Producer) {
	configMap := kafka.ConfigMap{
		"bootstrap.servers": boostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "kafka-to-blob",
	}

	var err error
	consumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	producer, err = kafka.NewProducer(&configMap)
	if err != nil {
		panic(err)
	}
	return
}

func processKafkaQueue(topic string) {
	//Usually ^ia.+
	zap.S().Debugf("Topic: %s", topic)
	err := kafkaConsumerClient.Subscribe(topic, nil)
	if err != nil {
		panic(err)
	}

	for !ShuttingDown {
		var msg *kafka.Message
		msg, err = kafkaConsumerClient.ReadMessage(5) //No infinitive timeout to be able to cleanly shut down
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			} else {
				zap.S().Warnf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		zap.S().Debugf("Key: %s", msg.Key)
		customerId, location, assetId, prefix, ignore := getIdsFromKey(msg.Key)
		if ignore {
			continue
		}
		zap.S().Debugf("CustomerID: %s, Location: %s, AssetId: %s, Prefix: %s", customerId, location, assetId, prefix)

		// TODO: This can only be done after DB rework
		switch prefix {
		case Prefix.ProcessValueFloat64:
		case Prefix.ProcessValue:
		case Prefix.ProcessValueString:
		case Prefix.Count:
		case Prefix.Recommendation:
		case Prefix.State:
		case Prefix.UniqueProduct:
		case Prefix.ScrapCount:
		case Prefix.AddShift:
		case Prefix.UniqueProductScrap:
		case Prefix.AddProduct:
		case Prefix.AddOrder:
		case Prefix.StartOrder:
		case Prefix.EndOrder:
		case Prefix.AddMaintenanceActivity:
		case Prefix.ProductTag:
		case Prefix.ProductTagString:
		case Prefix.AddParentToChild:
		case Prefix.ModifyState:
		case Prefix.ModifyProducesPieces:
		case Prefix.DeleteShiftById:
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
		default:
			zap.S().Debugf("Unknown prefix: %s", prefix)
			continue
		}
	}
}

func getIdsFromKey(key []byte) (customerId string, location string, assetId string, prefix string, ignore bool) {
	strkey := strings.Split(string(key), ".")
	ignore = false
	if len(strkey) != 5 {
		ignore = true
		return
	}

	//strkey[0] == ia
	customerId = strkey[1]
	location = strkey[2]
	assetId = strkey[3]
	prefix = strkey[4]
	return
}
