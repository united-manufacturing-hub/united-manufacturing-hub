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

var highThroughputProcessorChannel chan *kafka.Message
var highThroughputPutBackChannel chan internal.PutBackChanMsg

var HTKafkaConsumer *kafka.Consumer
var HTKafkaProducer *kafka.Producer
var HTKafkaAdminClient *kafka.AdminClient

func SetupHTKafka(configMap kafka.ConfigMap) {

	var err error
	HTKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaConsumer: %s", err)
	}

	HTKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaProducer: %s", err)
	}

	HTKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaAdminClient: %s", err)
	}

}

func CloseHTKafka() {

	zap.S().Infof("[HT]Closing Kafka Consumer")

	if err := HTKafkaConsumer.Close(); err != nil {
		zap.S().Fatalf("Failed to close KafkaConsumer: %s", err)
	}

	zap.S().Infof("[HT]Closing Kafka Producer")
	HTKafkaProducer.Flush(100)
	HTKafkaProducer.Close()

	zap.S().Infof("[HT]Closing Kafka Admin Client")
	HTKafkaAdminClient.Close()
}
