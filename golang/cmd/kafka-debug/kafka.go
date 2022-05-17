package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/umh-lib/v2/other"
	"go.uber.org/zap"
	"time"
)

func startDebugger() {
	kafka2.KafkaConsumer.Subscribe("^ia.+", nil)
	for !ShuttingDown {

		msg, err := kafka2.KafkaConsumer.ReadMessage(5) //No infinitive timeout to be able to cleanly shut down
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				// Sleep to reduce CPU usage
				time.Sleep(other.OneSecond)
				continue
			} else {
				zap.S().Errorf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		zap.S().Infof(" == Received message == ")
		zap.S().Infof("Topic: ", msg.TopicPartition.Topic)
		zap.S().Infof("Value: ", msg.Value)
	}
}
