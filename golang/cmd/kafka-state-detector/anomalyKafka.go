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

var AnomalyProcessorChannel chan *kafka.Message
var AnomalyCommitChannel chan *kafka.Message
var AnomalyPutBackChannel chan internal.PutBackChanMsg

// AnomalyKafkaConsumer is a high Integrity Kafka consumer
var AnomalyKafkaConsumer *kafka.Consumer

// AnomalyKafkaProducer is a high Integrity Kafka producer
var AnomalyKafkaProducer *kafka.Producer

// AnomalyKafkaAdminClient is a high Integrity Kafka admin
var AnomalyKafkaAdminClient *kafka.AdminClient

// SetupAnomalyKafka sets up the Anomaly Kafka consumer, producer and admin
func SetupAnomalyKafka(configMap kafka.ConfigMap) {
	if !AnomalyEnabled {
		return
	}

	var err error
	AnomalyKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create consumer: %s", err)
	}

	AnomalyKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create producer: %s", err)
	}

	AnomalyKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create new admin client: %s", err)
	}

}

// CloseAnomalyKafka closes the Anomaly Kafka consumer, producer and admin
func CloseAnomalyKafka() {
	if !AnomalyEnabled {
		return
	}
	zap.S().Infof("[Anomaly]Closing Kafka Consumer")
	err := AnomalyKafkaConsumer.Close()
	if err != nil {
		zap.S().Fatalf("[Anomaly]Failed to close Kafka Consumer: %s", err)
	}

	zap.S().Infof("[Anomaly]Closing Kafka Producer")
	AnomalyKafkaProducer.Flush(100)
	AnomalyKafkaProducer.Close()

	zap.S().Infof("[Anomaly]Closing Kafka Admin Client")
	AnomalyKafkaAdminClient.Close()
}

var kmsq kafkaMessageStreamQueue

func startAnomalyActivityProcessor() {
	kmsq = NewKafkaMessageStreamQueue(make([]datamodel.Activity, 0))
	for !ShuttingDown {

		// Get next message from HI kafka consumer
		msg := <-AnomalyProcessorChannel
		if msg == nil {
			continue
		}
		parsed, parsedMessage := internal.ParseMessage(msg)
		if !parsed {
			continue
		}

		switch parsedMessage.PayloadType {
		case "activity":
			{

				var activityMessage datamodel.Activity
				err := jsoniter.Unmarshal(parsedMessage.Payload, &activityMessage)
				if err != nil {
					zap.S().Errorf("[AN] Failed to parse activity message: %s", err)
					continue
				}

				kmsq.Enqueue(activityMessage)
				break
			}
		case "detectedAnomaly":
			{
				latestActivity := kmsq.GetLatestByTimestamp()
				if latestActivity.Activity {
					continue
				}
				var detectedAnomalyMessage datamodel.DetectedAnomaly
				err := jsoniter.Unmarshal(parsedMessage.Payload, &detectedAnomalyMessage)
				if err != nil {
					zap.S().Errorf("[AN] Failed to parse activity message: %s", err)
					continue
				}

				stateNumber := datamodel.GetStateFromString(detectedAnomalyMessage.DetectedAnomaly)
				if stateNumber == 0 {
					zap.S().Errorf(
						"[AN] Failed to get state number from string: %s",
						detectedAnomalyMessage.DetectedAnomaly)
					continue
				}

				var stateMessage datamodel.State
				stateMessage.State = stateNumber
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
				err = internal.Produce(AnomalyKafkaProducer, msgS, nil)
				if err != nil {
					errS := err.Error()
					AnomalyPutBackChannel <- internal.PutBackChanMsg{
						Msg:         msg,
						Reason:      "Failed to produce state message",
						ErrorString: &errS,
					}
					continue
				}
				AnomalyCommitChannel <- msg
				break
			}
		}

	}
}
