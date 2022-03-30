package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
)

var highIntegrityProcessorChannel chan *kafka.Message
var highIntegrityCommitChannel chan *kafka.Message
var highIntegrityPutBackChannel chan PutBackChanMsg

// DrainHIChannel empties a channel into the high integrity putback channel

var HIKafkaConsumer *kafka.Consumer
var HIKafkaProducer *kafka.Producer
var HIKafkaAdminClient *kafka.AdminClient

func SetupHIKafka(configMap kafka.ConfigMap) {
	if !HighIntegrityEnabled {
		return
	}

	var err error
	HIKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	HIKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		panic(err)
	}

	HIKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	return
}

func CloseHIKafka() {
	if !HighIntegrityEnabled {
		return
	}
	zap.S().Infof("[HI]Closing Kafka Consumer")
	err := HIKafkaConsumer.Close()
	if err != nil {
		panic("Failed do close HIKafkaConsumer client !")
	}

	zap.S().Infof("[HI]Closing Kafka Producer")
	HIKafkaProducer.Flush(100)
	HIKafkaProducer.Close()

	zap.S().Infof("[HI]Closing Kafka Admin Client")
	HIKafkaAdminClient.Close()
}

func GetHICommitOffset() (lowOffset int64, highOffset int64, err error) {
	if !HighIntegrityEnabled {
		return 0, 0, nil
	}

	lowOffset, highOffset, err = HIKafkaConsumer.GetWatermarkOffsets(HITopic, 0)
	if err != nil {
		zap.S().Infof("[HI]GetWatermarkOffsets errored: %s", err)
		return 0, 0, err
	}

	return lowOffset, highOffset, err
}
