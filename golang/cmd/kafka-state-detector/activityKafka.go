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
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

var ActivityProcessorChannel chan *kafka.Message
var ActivityCommitChannel chan *kafka.Message
var ActivityPutBackChannel chan internal.PutBackChanMsg

// ActivityKafkaConsumer is a high Integrity Kafka consumer
var ActivityKafkaConsumer *kafka.Consumer

// ActivityKafkaProducer is a high Integrity Kafka producer
var ActivityKafkaProducer *kafka.Producer

// ActivityKafkaAdminClient is a high Integrity Kafka admin
var ActivityKafkaAdminClient *kafka.AdminClient

// SetupActivityKafka sets up the Activity Kafka consumer, producer and admin
func SetupActivityKafka(configMap kafka.ConfigMap) {
	if !ActivityEnabled {
		return
	}

	var err error
	ActivityKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create consumer: %s", err)
	}

	ActivityKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create producer: %s", err)
	}

	ActivityKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create new admin client: %s", err)
	}

}

// CloseActivityKafka closes the Activity Kafka consumer, producer and admin
func CloseActivityKafka() {
	if !ActivityEnabled {
		return
	}
	zap.S().Infof("[Activity]Closing Kafka Consumer")
	err := ActivityKafkaConsumer.Close()
	if err != nil {
		zap.S().Fatalf("[Activity]Failed to close Kafka Consumer: %s", err)
	}

	zap.S().Infof("[Activity]Closing Kafka Producer")
	ActivityKafkaProducer.Flush(100)
	ActivityKafkaProducer.Close()

	zap.S().Infof("[Activity]Closing Kafka Admin Client")
	ActivityKafkaAdminClient.Close()
}

var lastStateChangeTs = uint64(0)

func startActivityProcessor() {
	for !ShuttingDown {
		// Get next message from HI kafka consumer
		msg := <-ActivityProcessorChannel
		if msg == nil {
			continue
		}
		parsed, parsedMessage := internal.ParseMessage(msg)
		if !parsed {
			continue
		}

		if parsedMessage.PayloadType != "activity" {
			continue
		}

		var activityMessage datamodel.Activity
		err := jsoniter.Unmarshal(parsedMessage.Payload, &activityMessage)
		if err != nil {
			zap.S().Errorf("[AC] Failed to parse activity message: %s", err)
			continue
		}

		var stateMessage datamodel.State
		if activityMessage.Activity {
			if activityMessage.TimestampMs == lastStateChangeTs {
				continue
			}
			stateMessage.State = datamodel.ProducingAtFullSpeedState
			lastStateChangeTs = activityMessage.TimestampMs
		} else {
			if activityMessage.TimestampMs == lastStateChangeTs {
				continue
			}
			stateMessage.State = datamodel.UnspecifiedStopState
			lastStateChangeTs = activityMessage.TimestampMs
		}

		jsonStateMessage, err := jsoniter.Marshal(stateMessage)
		if err != nil {
			continue
		}

		stateTopic := fmt.Sprintf(
			"ia.%s.%s.%s.state",
			parsedMessage.CustomerId,
			parsedMessage.Location,
			parsedMessage.AssetId)
		msgS := &kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &stateTopic, Partition: kafka.PartitionAny},
			Value:          jsonStateMessage,
		}
		err = internal.Produce(ActivityKafkaProducer, msgS, nil)
		if err != nil {
			errS := err.Error()
			ActivityPutBackChannel <- internal.PutBackChanMsg{
				Msg:         msg,
				Reason:      "Failed to produce state message",
				ErrorString: &errS,
			}
			continue
		}
		ActivityCommitChannel <- msg
	}
}
