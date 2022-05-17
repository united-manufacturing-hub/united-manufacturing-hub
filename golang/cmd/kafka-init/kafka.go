package main

import (
	kafka2 "github.com/united-manufacturing-hub/umh-lib/v2/kafka"
	"strings"
)

func initKafkaTopics(topics string) {
	topiclist := strings.Split(topics, ";")
	for _, topic := range topiclist {
		err := kafka2.CreateTopicIfNotExists(topic)
		if err != nil {
			panic(err)
		}
	}
}
