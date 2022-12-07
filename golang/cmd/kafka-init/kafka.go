package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"strings"
)

func initKafkaTopics(topics string) {
	zap.S().Debugf("Creating Kafka topics: %s", topics)

	topiclist := strings.Split(topics, ";")
	for _, topic := range topiclist {
		err := internal.CreateTopicIfNotExists(topic)
		if err != nil {
			zap.S().Fatalf("Failed to create topic %s: %s", topic, err)
		}
	}
}
