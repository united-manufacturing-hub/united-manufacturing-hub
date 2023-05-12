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
)

var highIntegrityProcessorChannel chan *kafka.Message
var highIntegrityCommitChannel chan *kafka.Message
var highIntegrityPutBackChannel chan PutBackChanMsg

// HIKafkaClient is a high Integrity Kafka client
var HIKafkaClient *kafka.Client

// SetupHIKafka sets up the HI Kafka consumer, producer and admin
func SetupHIKafka(opts kafka.NewClientOptions) {

	var err error
	HIKafkaClient, err = kafka.NewKafkaClient(opts)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaClient: %s", err)
	}

}

// CloseHIKafka closes the HI Kafka consumer, producer and admin
func CloseHIKafka() {

	zap.S().Infof("[HI]Closing Kafka Client")

	if err := HIKafkaClient.Close(); err != nil {
		zap.S().Fatalf("Failed to close HIKafkaClient: %s", err)
	}
}
