package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/kafka_helper"
	"strings"
)

func initKafkaTopics(topics string) {
	topiclist := strings.Split(topics, ";")
	for _, topic := range topiclist {
		err := kafka_helper.CreateTopicIfNotExists(topic)
		panic(err)
	}
}
