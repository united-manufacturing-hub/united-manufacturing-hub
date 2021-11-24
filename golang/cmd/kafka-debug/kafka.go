package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"time"
)

func setupKafka(boostrapServer string) (consumer *kafka.Consumer) {
	configMap := kafka.ConfigMap{
		"bootstrap.servers": boostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "mqtt-debug",
	}
	var err error
	consumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	return
}

func startDebugger() {
	kafkaConsumerClient.Subscribe("^ia.+", nil)
	for !ShuttingDown {

		msg, err := kafkaConsumerClient.ReadMessage(5) //No infinitive timeout to be able to cleanly shut down
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
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
