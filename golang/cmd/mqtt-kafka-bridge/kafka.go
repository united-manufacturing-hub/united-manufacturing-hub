package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
)

func setupKafka(boostrapServer string) (producer *kafka.Producer) {
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"boostrap.servers": boostrapServer,
	})

	if err != nil {
		panic(err)
	}

	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					zap.S().Errorf("Delivery failed: %v (%s)", ev.TopicPartition, ev.TopicPartition.Error)
				} else {
					zap.S().Infof("Delivered message to %v", ev.TopicPartition)
				}
			}
		}
	}()

	return
}
