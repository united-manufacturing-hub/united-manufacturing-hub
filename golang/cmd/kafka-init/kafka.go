package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"strings"
)

func initKafkaTopics(topics string) {
	topiclist := strings.Split(topics, ";")
	for _, topic := range topiclist {
		err := internal.CreateTopicIfNotExists(topic)
		panic(err)
	}
}
