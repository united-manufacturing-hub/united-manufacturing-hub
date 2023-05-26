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

var highThroughputProcessorChannel chan *kafka.Message
var highThroughputPutBackChannel chan PutBackChanMsg

var HTKafkaClient *kafka.Client

func SetupHTKafka(opts *kafka.NewClientOptions) {

	var err error
	HTKafkaClient, err = kafka.NewKafkaClient(opts)
	if err != nil {
		zap.S().Fatalf("Failed to create HTKafkaClient: %s", err)
	}

}

func CloseHTKafka() {

	zap.S().Infof("[HT]Closing Kafka Client")

	if err := HTKafkaClient.Close(); err != nil {
		zap.S().Fatalf("Failed to close HTKafkaClient: %s", err)
	}
}
