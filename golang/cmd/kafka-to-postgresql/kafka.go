package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

var processorChannel chan *kafka.Message

func processKafkaQueue(topic string, channelCapacity int) {
	processorChannel = make(chan *kafka.Message, channelCapacity)
	err := internal.KafkaConsumer.Subscribe(topic, nil)
	if err != nil {
		panic(err)
	}

	for !ShuttingDown {
		var msg *kafka.Message
		msg, err = internal.KafkaConsumer.ReadMessage(5)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			} else if err.(kafka.Error).Code() == kafka.ErrUnknownTopicOrPart {
				time.Sleep(5 * time.Second)
				continue
			} else {
				zap.S().Warnf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		processorChannel <- msg
	}
}

func CommitMessage(msg *kafka.Message) {
	_, err := internal.KafkaConsumer.CommitMessage(msg)
	if err != nil {
		zap.S().Errorf("Failed to commit message: %v (%s)", msg, err)
	}
}

func PutBack(msg *kafka.Message) {
	err := internal.KafkaProducer.Produce(msg, nil)
	if err != nil {
		zap.S().Errorf("Failed to put back message: %v (%s)", msg, err)
	}
}

func CleanProcessorChannel() {
	select {
	case msg, ok := <-processorChannel:
		if ok {
			PutBack(msg)
		} else {
			zap.S().Debugf("Processor channel is closed !")
			return
		}
	default:
		{
			zap.S().Debugf("Processor channel is empty !")
			return
		}
	}
}
