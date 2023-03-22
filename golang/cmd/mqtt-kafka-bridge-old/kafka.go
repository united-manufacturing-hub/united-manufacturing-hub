// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
		internal.SleepBackedOff(loopsSinceLastMessage, 1*time.Nanosecond, 1*time.Second)
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

	var msg *kafka.Message
	for !ShuttingDown {
		time.Sleep(time.Duration(mqttOutGoingQueue.Length()) * time.Millisecond)
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
		go processMessage(msg)
	}
	zap.S().Infof("Kafka consumer for topic %s stopped", topic)
}

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func processMessage(msg *kafka.Message) {
	kafkaTopic := *msg.TopicPartition.Topic

	if !strings.HasPrefix(kafkaTopic, "ia.raw") {
		// Ignore every message from the same producer.
		trace := internal.GetTrace(msg, "x-trace")
		if trace != nil {
			foundTrace := false
			for _, v := range trace.Traces {
				if v == fmt.Sprintf("%s-%s", internal.MicroserviceName, internal.SerialNumber) {
					pathTraces := internal.GetTrace(msg, "x-trace")
					zap.S().Debugf("Skipping message from own producer & cluster")
					zap.S().Debugf("Path traces: %v", pathTraces)
					// Ignore messages that our cluster created
					foundTrace = true
					break
				}
			}
			if foundTrace {
				return
			}
		} else if !KafkaAcceptNoOrigin {
			zap.S().Debugf("Skipping message without origin")
			// Ignore messages without defined origin !
			return
		}
	}

	payload := msg.Value

	if json.Valid(payload) || strings.HasPrefix(kafkaTopic, "ia.raw") {
		validTopic, mqttTopic := internal.KafkaTopicToMqtt(kafkaTopic)

		if validTopic {
			go storeNewMessageIntoQueue(mqttTopic, payload, mqttOutGoingQueue)
		} else {
			zap.S().Warnf("kafkaToQueue [INVALID] message not forwarded because the topic is not valid (%s) [%s]", kafkaTopic, payload)
		}
	} else {
		zap.S().Warnf(
			"kafkaToQueue [INVALID] message not forwarded because the content is not a valid JSON (%s) [%s]",
			kafkaTopic, payload)
	}
}
