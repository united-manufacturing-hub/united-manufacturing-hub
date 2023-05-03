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
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
)

// SendKafkaMessage tries to send a message via kafka
func SendKafkaMessage(kafkaProducerClient *kafka.Client, kafkaTopicName string, message []byte) {
	if !useKafka {
		return
	}

	messageHash := xxh3.Hash(message)
	cacheKey := fmt.Sprintf("SendKafkaMessage%s%d", kafkaTopicName, messageHash)

	_, found := internal.GetMemcached(cacheKey)
	if found {
		zap.S().Debugf(
			"Duplicate message for topic %s, you might want to increase LOWER_POLLING_TIME !",
			kafkaTopicName)
		return
	}

	err := internal.CreateTopicIfNotExists(kafkaTopicName)
	if err != nil {
		zap.S().Fatal("Failed to create topic %s", err)
	}

	if kafkaProducerClient == nil {
		zap.S().Fatal("Received kafka producer is empty!!")
	}

	err = kafkaProducerClient.EnqueueMessage(kafka.Message{
		Topic: kafkaTopicName,
		Value: message,
	})

	if err != nil {
		zap.S().Errorf("Failed to send Kafka message: %s", err)
	} else {
		internal.SetMemcached(cacheKey, nil)
	}
}

// setupKafka sets up the connection to the kafka server
func setupKafka(client *kafka.Client, boostrapServer string) {
	if !useKafka {
		return
	}
	useSsl, _ := env.GetAsBool("KAFKA_USE_SSL", false, false)
	var err error
	client, err = kafka.NewKafkaClient(kafka.NewClientOptions{
		Brokers: []string{
			boostrapServer,
		},
		ConsumerName:      "sensorconnect",
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         useSsl,
		StartOffset:       sarama.OffsetOldest,
	})

	if err != nil {
		zap.S().Fatalf("Failed to create kafka producer: %s", err)
	}

	return
}
