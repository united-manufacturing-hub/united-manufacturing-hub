package main

import (
	"encoding/json"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

func processIncomingMessages() {
	var err error

	for !ShuttingDown {
		if mqttIncomingQueue.Length() == 0 {
			//Skip if empty
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var object queueObject
		object, err = retrieveMessageFromQueue(mqttIncomingQueue)
		if err != nil {
			zap.S().Errorf("Failed to dequeue message: %s", err)
			time.Sleep(1 * time.Second)
			continue
		}

		//Setup Topic if not exist
		validTopic, kafkaTopicName := internal.MqttTopicToKafka(object.Topic)
		if !validTopic {
			continue
		}
		err = internal.CreateTopicIfNotExists(kafkaTopicName)
		if err != nil {
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			continue
		}

		zap.S().Debugf("Sending with Topic: %s", kafkaTopicName)
		err = internal.KafkaProducer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &kafkaTopicName,
				Partition: kafka.PartitionAny,
			},
			Value: object.Message,
		}, nil)
		if err != nil {
			zap.S().Errorf("Failed to send Kafka message: %s", err)
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			continue
		}
	}
}

func kafkaToQueue(topic string) {
	err := internal.KafkaConsumer.Subscribe(topic, nil)
	if err != nil {
		panic(err)
	}

	for !ShuttingDown {
		msg, err := internal.KafkaConsumer.ReadMessage(5) //No infinitive timeout to be able to cleanly shut down
		if err != nil {
			// This is fine, and expected behaviour
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				// Sleep to reduce CPU usage
				time.Sleep(internal.OneSecond)
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

		payload := msg.Value
		if json.Valid(payload) {
			kafkaTopic := msg.TopicPartition.Topic
			validTopic, mqttTopic := internal.KafkaTopicToMqtt(*kafkaTopic)

			if validTopic {
				go storeNewMessageIntoQueue(mqttTopic, payload, mqttOutGoingQueue)
				zap.S().Debugf("kafkaToQueue", topic, payload)
			}
		} else {
			zap.S().Warnf("kafkaToQueue [INVALID] message not forwarded because the content is not a valid JSON", topic, payload)
		}
	}
}
