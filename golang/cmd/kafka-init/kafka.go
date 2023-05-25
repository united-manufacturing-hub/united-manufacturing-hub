// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"github.com/Shopify/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"strings"
)

func Init(kafkaBroker string) {
	kafkaTopics, err := env.GetAsString("KAFKA_TOPICS", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("kafkaTopics: %s", kafkaTopics)

	topicList := strings.Split(kafkaTopics, ";")

	useSsl, err := env.GetAsBool("KAFKA_USE_SSL", false, false)
	if err != nil {
		zap.S().Error(err)
	}

	zap.S().Debug("Creating kafka client")
	client, err := kafka.NewKafkaClient(&kafka.NewClientOptions{
		Brokers:           []string{kafkaBroker},
		ConsumerName:      "kafka-init",
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         useSsl,
		StartOffset:       sarama.OffsetOldest,
	})
	if err != nil {
		zap.S().Fatalf("Error creating kafka client: %v", err)
		return
	}
	defer func(client *kafka.Client) {
		zap.S().Debug("Closing kafka client")
		err = client.Close()
		if err != nil {
			zap.S().Fatalf("Error closing kafka client: %v", err)
		}
	}(client)

	initKafkaTopics(client, topicList)

	err = client.Close()
	if err != nil {
		zap.S().Fatalf("Error closing kafka client: %v", err)
	}
}

func initKafkaTopics(client *kafka.Client, topicList []string) {
	for _, topic := range topicList {
		zap.S().Debugf("Creating topic %s", topic)
		err := client.TopicCreator(topic)
		if err != nil {
			zap.S().Errorf("Error creating topic %s: %v", topic, err)
		}
	}
}
