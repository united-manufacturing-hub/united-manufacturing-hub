package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
)

var highIntegrityProcessorChannel chan *kafka.Message
var highIntegrityCommitChannel chan *kafka.Message
var highIntegrityPutBackChannel chan PutBackChanMsg

// HIKafkaConsumer is a high Integrity Kafka consumer
var HIKafkaConsumer *kafka.Consumer

// HIKafkaProducer is a high Integrity Kafka producer
var HIKafkaProducer *kafka.Producer

// HIKafkaAdminClient is a high Integrity Kafka admin
var HIKafkaAdminClient *kafka.AdminClient

// SetupHIKafka sets up the HI Kafka consumer, producer and admin
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

// CloseHIKafka closes the HI Kafka consumer, producer and admin
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
