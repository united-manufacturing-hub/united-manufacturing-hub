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
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

func startDebugger() {
	err := internal.KafkaConsumer.Subscribe("^ia.+", nil)
	if err != nil {
		zap.S().Fatalf("Failed to subscribe to kafka topic: %s", err)
	}
	for !ShuttingDown {

		msg, err := internal.KafkaConsumer.ReadMessage(5) // No infinitive timeout to be able to cleanly shut down
		if err != nil {
			var kafkaError kafka.Error
			ok := errors.As(err, &kafkaError)

			if ok && kafkaError.Code() == kafka.ErrTimedOut {
				// Sleep to reduce CPU usage
				time.Sleep(internal.OneSecond)
				continue
			} else {
				zap.S().Errorf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		zap.S().Infof(" == Received message == ")
		zap.S().Infof("Topic: ", msg.TopicPartition.Topic)
		zap.S().Infof("Value: ", msg.Value)
	}
}
