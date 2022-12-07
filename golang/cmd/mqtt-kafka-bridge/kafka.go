package main

import (
	"errors"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"strings"
	"time"
)

var sn = os.Getenv("SERIAL_NUMBER")

var KafkaSenderThreads int
var KafkaAcceptNoOrigin bool

func processIncomingMessages() {
	for i := 0; i < KafkaSenderThreads; i++ {
		go SendKafkaMessages()
	}
}

func SendKafkaMessages() {
	var err error

	for !ShuttingDown {
		if mqttIncomingQueue.Length() == 0 {
			// Skip if empty
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var object queueObject
		var gotItem bool
		object, err, gotItem = retrieveMessageFromQueue(mqttIncomingQueue)
		if err != nil {
			if !gotItem {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			zap.S().Errorf("Failed to dequeue message: %s", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// Setup Topic if not exist
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

		msg := &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &kafkaTopicName,
				Partition: kafka.PartitionAny,
			},
			Value: object.Message,
		}
		err = internal.AddTrace(msg, "x-origin", sn)
		if err != nil {
			zap.S().Errorf("Failed to add trace: %s", err)
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			continue
		}
		err = internal.Produce(internal.KafkaProducer, msg, nil, fmt.Sprintf("mqtt-kafka-bridge-%s", sn))

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
		zap.S().Fatalf("Failed to subscribe to topic %s: %s", topic, err)
	}

	stuck := 0

	var msg *kafka.Message
	for !ShuttingDown {
		if mqttOutGoingQueue.Length() >= 100 {
			// MQTT can't keep up with Kafka
			time.Sleep(internal.OneSecond)
			stuck += 1

			// If we are stuck for more than 60 seconds, then we need to shut down
			if stuck > 60 {
				// MQTT seems down, restart app
				ShutdownApplicationGraceful()
				return
			}
			continue
		}
		stuck = 0
		msg, err = internal.KafkaConsumer.ReadMessage(5) // No infinitive timeout to be able to cleanly shut down
		if err != nil {
			// This is fine, and expected behaviour
			var kafkaError kafka.Error
			ok := errors.As(err, &kafkaError)
			if ok && kafkaError.Code() == kafka.ErrTimedOut {
				// Sleep to reduce CPU usage
				time.Sleep(internal.OneSecond)
				continue
			} else if ok && kafkaError.Code() == kafka.ErrUnknownTopicOrPart {
				time.Sleep(5 * time.Second)
				continue
			} else {
				zap.S().Warnf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		trace := internal.GetTrace(msg, "x-origin")
		if trace != nil {
			for _, v := range trace.Traces {
				if v == sn {
					// Ignore messages that our cluster created
					continue
				}
			}
		} else if !KafkaAcceptNoOrigin {
			// Ignore messages without defined origin !
			continue
		}

		payload := msg.Value
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		kafkaTopic := *msg.TopicPartition.Topic
		if json.Valid(payload) || strings.HasPrefix(kafkaTopic, "ia.raw") {
			validTopic, mqttTopic := internal.KafkaTopicToMqtt(kafkaTopic)

			if validTopic {
				go storeNewMessageIntoQueue(mqttTopic, payload, mqttOutGoingQueue)
				zap.S().Debugf("kafkaToQueue (%s) (%s)", kafkaTopic, payload)
			}
		} else {
			zap.S().Warnf(
				"kafkaToQueue [INVALID] message not forwarded because the content is not a valid JSON (%s) [%s]",
				kafkaTopic, payload)
		}
	}
}
