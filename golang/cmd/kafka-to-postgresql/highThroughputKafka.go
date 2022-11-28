package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

var highThroughputProcessorChannel chan *kafka.Message
var highThroughputPutBackChannel chan internal.PutBackChanMsg

var HTKafkaConsumer *kafka.Consumer
var HTKafkaProducer *kafka.Producer
var HTKafkaAdminClient *kafka.AdminClient

func SetupHTKafka(configMap kafka.ConfigMap) {

	var err error
	HTKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		zap.S().Fatalf("Error: %s", err)
	}

	HTKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		zap.S().Fatalf("Error: %s", err)
	}

	HTKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		zap.S().Fatalf("Error: %s", err)
	}

}

func CloseHTKafka() {

	zap.S().Infof("[HT]Closing Kafka Consumer")

	if err := HTKafkaConsumer.Close(); err != nil {
		zap.S().Fatal
	}

	zap.S().Infof("[HT]Closing Kafka Producer")
	HTKafkaProducer.Flush(100)
	HTKafkaProducer.Close()

	zap.S().Infof("[HT]Closing Kafka Admin Client")
	HTKafkaAdminClient.Close()
}
