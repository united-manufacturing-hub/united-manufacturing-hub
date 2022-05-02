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
	if !HighThroughputEnabled {
		return
	}

	var err error
	HTKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	HTKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		panic(err)
	}

	HTKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

}

func CloseHTKafka() {
	if !HighThroughputEnabled {
		return
	}
	zap.S().Infof("[HT]Closing Kafka Consumer")
	err := HTKafkaConsumer.Close()
	if err != nil {
		panic("Failed do close HTKafkaConsumer client !")
	}

	zap.S().Infof("[HT]Closing Kafka Producer")
	HTKafkaProducer.Flush(100)
	HTKafkaProducer.Close()

	zap.S().Infof("[HT]Closing Kafka Admin Client")
	HTKafkaAdminClient.Close()
}
