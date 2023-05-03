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
	"github.com/Shopify/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"regexp"
)

var client *kafka.Client

func startDebugger() {
	if client != nil {
		return
	}

	// Read environment variables for Kafka
	KafkaBootstrapServer, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	//Parsing the topic to subscribe
	compile, err := regexp.Compile("^ia.+")
	if err != nil {
		zap.S().Fatalf("Error compiling regex: %v", err)
	}
	client, err = kafka.NewKafkaClient(kafka.NewClientOptions{
		Brokers: []string{
			KafkaBootstrapServer,
		},
		ConsumerName:      "kafka-debug",
		ListenTopicRegex:  compile,
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         true,
		StartOffset:       sarama.OffsetOldest,
	})
	if err != nil {
		zap.S().Fatalf("Failed to subscribe to kafka topic: %s", err)
	}
	for {

		msg := <-client.GetMessages()

		zap.S().Infof(" == Received message == ")
		zap.S().Infof("Topic: ", msg.Topic)
		zap.S().Infof("Value: ", msg.Value)
	}
}

func Shutdown() {
	zap.S().Info("Shutting down kafka client")
	err := client.Close()
	if err != nil {
		zap.S().Fatalf("Error closing kafka client: %v", err)
	}
	zap.S().Info("Kafka client shut down")
}
