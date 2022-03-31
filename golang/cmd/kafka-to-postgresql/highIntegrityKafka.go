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
