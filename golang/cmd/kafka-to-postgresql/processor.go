package main

import "github.com/confluentinc/confluent-kafka-go/kafka"

func queueProcessor() {
	for !ShuttingDown {
		var msg *kafka.Message
		msg = <-processorChannel
		msg.Value
	}
}
