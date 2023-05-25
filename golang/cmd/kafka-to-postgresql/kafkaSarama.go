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
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"go.uber.org/zap"
	"regexp"
)

var KafkaClient *kafka.Client
var KafkaTopicProbeConsumer *kafka.Client

var probeTopicName = "umh.v1.kafka.newTopic"

func SetupKafka(opts *kafka.NewClientOptions) {
	zap.S().Debugf("Options: %v", opts)

	var err error
	KafkaClient, err = kafka.NewKafkaClient(opts)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaConsumer: %s", err)
	}

	zap.S().Debugf("KafkaClient: %+v", KafkaClient)

}

// SetupKafkaTopicProbeConsumer sets up a consumer for detecting new topics
func SetupKafkaTopicProbeConsumer(opts *kafka.NewClientOptions) {
	zap.S().Debugf("Options: %v", opts)

	var err error
	KafkaTopicProbeConsumer, err = kafka.NewKafkaClient(opts)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaTopicProbeConsumer: %s", err)
	}

	//Parsing the topic to subscribe
	compile, err := regexp.Compile(probeTopicName)
	if err != nil {
		zap.S().Fatalf("Error compiling regex: %v", err)
	}

	KafkaTopicProbeConsumer.ChangeSubscribedTopics(compile)

	zap.S().Debugf("KafkaTopicProbeConsumer: %+v", KafkaTopicProbeConsumer)
}

func CloseKafka() {
	if err := KafkaClient.Close(); err != nil {
		zap.S().Fatalf("Failed to close KafkaConsumer: %s", err)
	}
}

func CloseKafkaTopicProbeConsumer() {
	err := KafkaTopicProbeConsumer.Close()
	if err != nil {
		zap.S().Fatalf("Failed to close KafkaTopicProbeConsumer: %s", err)
	}
}
