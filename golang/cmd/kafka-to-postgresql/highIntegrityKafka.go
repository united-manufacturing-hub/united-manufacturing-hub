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
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

var highIntegrityProcessorChannel chan *kafka.Message
var highIntegrityCommitChannel chan *kafka.Message
var highIntegrityPutBackChannel chan internal.PutBackChanMsg

// HIKafkaConsumer is a high Integrity Kafka consumer
var HIKafkaConsumer *kafka.Consumer

// HIKafkaProducer is a high Integrity Kafka producer
var HIKafkaProducer *kafka.Producer

// HIKafkaAdminClient is a high Integrity Kafka admin
var HIKafkaAdminClient *kafka.AdminClient

// SetupHIKafka sets up the HI Kafka consumer, producer and admin
func SetupHIKafka(configMap kafka.ConfigMap) {

	var err error
	HIKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaConsumer: %s", err)
	}

	HIKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaProducer: %s", err)
	}

	HIKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaAdminClient: %s", err)
	}

}

// CloseHIKafka closes the HI Kafka consumer, producer and admin
func CloseHIKafka() {

	zap.S().Infof("[HI]Closing Kafka Consumer")

	if err := HIKafkaConsumer.Close(); err != nil {
		zap.S().Fatalf("Failed to close KafkaConsumer: %s", err)
	}

	zap.S().Infof("[HI]Closing Kafka Producer")
	HIKafkaProducer.Flush(100)
	HIKafkaProducer.Close()

	zap.S().Infof("[HI]Closing Kafka Admin Client")
	HIKafkaAdminClient.Close()
}
