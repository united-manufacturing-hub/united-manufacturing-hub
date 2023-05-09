package processor

import (
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-bridge/message"
	"go.uber.org/zap"
	"regexp"
)

var client1 *kafka.Client
var client2 *kafka.Client

func CreateClient(listenTopicRegex *regexp.Regexp, client1Options, client2Options kafka.NewClientOptions, kafkaChan chan kafka.Message) {
	client1Options.ListenTopicRegex = listenTopicRegex
	client2Options.ListenTopicRegex = listenTopicRegex

	var err error
	client1, err = kafka.NewKafkaClient(client1Options)
	if err != nil {
		zap.S().Fatalf("Error creating local kafka client: %v", err)
	}
	client2, err = kafka.NewKafkaClient(client2Options)
	if err != nil {
		zap.S().Fatalf("Error creating remote kafka client: %v", err)
	}

	go processIncomingMessage(kafkaChan)
}

func processIncomingMessage(kafkaChan chan kafka.Message) {
	for {
		msg := <-client1.GetMessages()
		kafkaChan <- kafka.Message{
			Topic:  msg.Topic,
			Value:  msg.Value,
			Header: msg.Header,
			Key:    msg.Key,
		}
	}
}

func Start(kafkaChan chan kafka.Message) {
	for {
		msg := <-kafkaChan
		if !message.IsValidKafkaMessage(msg) {
			continue
		}
		err := client2.EnqueueMessage(msg)
		if err != nil {
			zap.S().Errorf("Error sending message to kafka: %v", err)
		}
	}
}

func Shutdown() {
	zap.S().Info("Shutting down kafka clients")
	err := client1.Close()
	if err != nil {
		zap.S().Errorf("Error closing kafka client: %v", err)
	}
	err = client2.Close()
	if err != nil {
		zap.S().Errorf("Error closing kafka client: %v", err)
	}
	zap.S().Info("Kafka clients shut down")
}
func GetStats() (sent uint64, received uint64) {
	return kafka.GetKafkaStats()
}
