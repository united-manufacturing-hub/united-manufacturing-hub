package main

import (
	"errors"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"strings"
	"time"
)

var KafkaSenderThreads int
var KafkaAcceptNoOrigin bool

func processIncomingMessages() {
	for i := 0; i < KafkaSenderThreads; i++ {
		go SendKafkaMessages()
	}
}

func SendKafkaMessages() {
	var err error

	var loopsSinceLastMessage = int64(0)
	for !ShuttingDown {
		internal.SleepBackedOff(loopsSinceLastMessage, 1*time.Millisecond, 1*time.Second)
		loopsSinceLastMessage += 1

		var object *queueObject
		var gotItem bool
		object, err, gotItem = retrieveMessageFromQueue(mqttIncomingQueue)
		if err != nil {
			if !gotItem {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			zap.S().Errorf("Failed to dequeue message: %s", err)
			continue
		}
		if object == nil {
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
		err = internal.Produce(internal.KafkaProducer, msg, nil)

		if err != nil {
			zap.S().Errorf("Failed to send Kafka message: %s", err)
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			continue
		}
		loopsSinceLastMessage = 0
	}
}

func kafkaToQueue(topic string) {
	zap.S().Infof("Starting Kafka consumer for topic: %s", topic)
	err := internal.KafkaConsumer.Subscribe(topic, nil)
	if err != nil {
		zap.S().Fatalf("Failed to subscribe to topic %s: %s", topic, err)
	}

	stuck := 0
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	var loopsSinceLastMessage = int64(0)
	var msg *kafka.Message
	for !ShuttingDown {
		internal.SleepBackedOff(loopsSinceLastMessage, 1*time.Millisecond, 1*time.Second)
		loopsSinceLastMessage += 1
		if mqttOutGoingQueue.Length() >= 100 {
			// MQTT can't keep up with Kafka
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
		msg, err = internal.KafkaConsumer.ReadMessage(10 * time.Millisecond) // No infinitive timeout to be able to cleanly shut down
		if err != nil {
			// This is fine, and expected behaviour
			var kafkaError kafka.Error
			ok := errors.As(err, &kafkaError)
			if ok && kafkaError.Code() == kafka.ErrTimedOut {
				continue
			} else if ok && kafkaError.Code() == kafka.ErrUnknownTopicOrPart {
				zap.S().Warnf("Topic %s does not exist", topic)
				continue
			} else {
				zap.S().Errorf("Kafka consumer failed: %s", err)
				zap.S().Warnf("Failed to read kafka message: %s", err)
				continue
			}
		}
		zap.S().Debugf("Received Kafka message: %v (%v)", msg, err)

		// Ignore every message from the same producer.
		trace := internal.GetTrace(msg, "x-trace")
		zap.S().Debugf("Trace: %v", trace)
		zap.S().Debugf("MicroserviceName: %s, SerialNumber: %s", internal.MicroserviceName, internal.SerialNumber)
		if trace != nil {
			foundTrace := false
			for _, v := range trace.Traces {
				zap.S().Debugf("Kafka trace: %s vs %s-%s", v, internal.MicroserviceName, internal.SerialNumber)
				if v == fmt.Sprintf("%s-%s", internal.MicroserviceName, internal.SerialNumber) {
					zap.S().Debugf("Skipping message from own producer & cluster")
					pathTraces := internal.GetTrace(msg, "x-trace")
					zap.S().Debugf("Path traces: %v", pathTraces)
					// Ignore messages that our cluster created
					foundTrace = true
					break
				}
			}
			if foundTrace {
				continue
			}
		} else if !KafkaAcceptNoOrigin {
			zap.S().Debugf("Skipping message without origin")
			// Ignore messages without defined origin !
			continue
		}

		payload := msg.Value

		kafkaTopic := *msg.TopicPartition.Topic
		if json.Valid(payload) || strings.HasPrefix(kafkaTopic, "ia.raw") {
			validTopic, mqttTopic := internal.KafkaTopicToMqtt(kafkaTopic)

			if validTopic {
				go storeNewMessageIntoQueue(mqttTopic, payload, mqttOutGoingQueue)
				zap.S().Debugf("kafkaToQueue (%s) (%s)", kafkaTopic, payload)
				loopsSinceLastMessage = 0
			}
		} else {
			zap.S().Warnf(
				"kafkaToQueue [INVALID] message not forwarded because the content is not a valid JSON (%s) [%s]",
				kafkaTopic, payload)
		}
	}
	zap.S().Infof("Kafka consumer for topic %s stopped", topic)
}
