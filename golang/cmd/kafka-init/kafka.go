package main

import (
	"github.com/Shopify/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"strings"
)

func Init(kafkaBroker string) {
	kafkaTopics, err := env.GetAsString("KAFKA_TOPICS", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("kafkaTopics: %s", kafkaTopics)

	topicList := strings.Split(kafkaTopics, ";")

	useSsl, _ := env.GetAsBool("KAFKA_USE_SSL", false, false)

	client, err := kafka.NewKafkaClient(kafka.NewClientOptions{
		Brokers:           []string{kafkaBroker},
		ConsumerName:      "kafka-init",
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         useSsl,
		StartOffset:       sarama.OffsetOldest,
	})
	if err != nil {
		zap.S().Fatalf("Error creating kafka client: %v", err)
		return
	}
	zap.S().Infof("client: %v", client)

	initKafkaTopics(client, topicList)
}

func initKafkaTopics(client *kafka.Client, topicList []string) {
	for _, topic := range topicList {
		zap.S().Infof("Creating topic %s", topic)
		err := client.TopicCreator(topic)
		if err != nil {
			zap.S().Errorf("Error creating topic %s: %v", topic, err)
		}
	}
}
