package processor

import (
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
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
		err := client2.EnqueueMessage(msg)
		if err != nil {
			zap.S().Errorf("Error sending message to kafka: %v", err)
		}
	}
}
