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
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"os"
)

// SendKafkaMessage tries to send a message via kafka
func SendKafkaMessage(kafkaTopicName string, message []byte) {
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

	err = internal.Produce(kafkaProducerClient, &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &kafkaTopicName,
			Partition: kafka.PartitionAny,
		},
		Value: message,
	}, nil)

	if err != nil {
		zap.S().Errorf("Failed to send Kafka message: %s", err)
	} else {
		internal.SetMemcached(cacheKey, nil)
	}
}

// setupKafka sets up the connection to the kafka server
func setupKafka(boostrapServer string) (producer *kafka.Producer, adminClient *kafka.AdminClient) {
	if !useKafka {
		return
	}
	securityProtocol := "plaintext"
	useSsl, err := env.GetAsBool("KAFKA_USE_SSL", false, false)
	if err != nil {
		zap.S().Error(err)
	}
	if useSsl {
		securityProtocol = "ssl"

		_, err = os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatalf("Failed to open /SSL_certs/kafka/tls.key: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Failed to open /SSL_certs/kafka/tls.crt: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Failed to open ca.crt: %s", err)
		}
	}

	kafkaSslPassword, err := env.GetAsString("KAFKA_SSL_KEY_PASSWORD", false, "")
	if err != nil {
		zap.S().Error(err)
	}
	configMap := kafka.ConfigMap{
		"security.protocol":        securityProtocol,
		"ssl.key.location":         "/SSL_certs/kafka/tls.key",
		"ssl.key.password":         kafkaSslPassword,
		"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
		"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
		"bootstrap.servers":        boostrapServer,
		"group.id":                 "sensorconnect",
	}
	producer, err = kafka.NewProducer(&configMap)

	if err != nil {
		zap.S().Fatalf("Failed to create kafka producer: %s", err)
	}

	adminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create Admin client: %s", err)
	}

	internal.KafkaProducer = producer
	internal.KafkaAdminClient = adminClient

	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					zap.S().Errorf("Delivery failed: %v (%s)", ev.TopicPartition, ev.TopicPartition.Error)
				}
			}
		}
	}()

	return
}
