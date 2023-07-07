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

package internal

import (
	"context"
	"errors"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/goccy/go-json"
	"go.uber.org/zap"
	"strings"
	"time"
)

var KafkaConsumer *kafka.Consumer
var KafkaProducer *kafka.Producer
var KafkaAdminClient *kafka.AdminClient
var KafkaTopicProbeConsumer *kafka.Consumer

var probeTopicName = "umh.v1.kafka.newTopic"

func SetupKafka(configMap kafka.ConfigMap) {
	zap.S().Debugf("Configmap: %v", configMap)

	var err error
	KafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaConsumer: %s", err)
	}

	KafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaProducer: %s", err)
	}

	KafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaAdminClient: %s", err)
	}
	zap.S().Debugf("KafkaConsumer: %+v", KafkaConsumer)
	zap.S().Debugf("KafkaProducer: %+v", KafkaProducer)
	zap.S().Debugf("KafkaAdminClient: %+v", KafkaAdminClient)

}

// SetupKafkaTopicProbeConsumer sets up a consumer for detecting new topics
func SetupKafkaTopicProbeConsumer(configMap kafka.ConfigMap) {
	zap.S().Debugf("Configmap: %v", configMap)

	var err error
	KafkaTopicProbeConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		zap.S().Fatalf("Failed to create KafkaTopicProbeConsumer: %s", err)
	}

	err = KafkaTopicProbeConsumer.Subscribe(probeTopicName, nil)
	if err != nil {
		tempAdmin, errX := kafka.NewAdminClient(&configMap)
		if errX != nil {
			zap.S().Errorf("Failed to create KafkaAdminClient: %s", errX)
			zap.S().Fatalf("Failed to subscribe to topic %s: %s", probeTopicName, err)
		}
		_, errX = tempAdmin.CreateTopics(context.Background(), []kafka.TopicSpecification{
			{
				Topic: probeTopicName,
				// Kafka topic probe, only needs one partition.
				// While the default for other topics is 6, it's only used for faster detection of new topics.
				// Kafka listeners will eventually detect the new topic, even if they are not listening to the probe topic.
				NumPartitions: 1,
			},
		})
		if errX != nil {
			zap.S().Errorf("Failed to create topic %s: %s", probeTopicName, errX)
			zap.S().Fatalf("Failed to subscribe to topic %s: %s", probeTopicName, err)
		}

		zap.S().Fatalf("Restarting service, to subscribe to new topic %s", probeTopicName)
	}

	zap.S().Debugf("KafkaTopicProbeConsumer: %+v", KafkaTopicProbeConsumer)
}

func CloseKafka() {

	if err := KafkaConsumer.Close(); err != nil {
		zap.S().Fatalf("Failed to close KafkaConsumer: %s", err)
	}

	KafkaProducer.Flush(100)
	KafkaProducer.Close()

	KafkaAdminClient.Close()
}

func CloseKafkaTopicProbeConsumer() {
	err := KafkaTopicProbeConsumer.Close()
	if err != nil {
		zap.S().Fatalf("Failed to close KafkaTopicProbeConsumer: %s", err)
	}
}

var lastMetaData *kafka.Metadata

func TopicExists(kafkaTopicName string) (exists bool, err error) {
	// Check if lastMetaData was initialized
	if lastMetaData == nil {
		// Get initial map of metadata
		lastMetaData, err = GetMetaData()
		if err != nil {
			zap.S().Errorf("Failed to get Kafka metadata: %s", err)
			return false, err
		}
	}

	// Check if current metadata cache has topic listed
	if _, ok := lastMetaData.Topics[kafkaTopicName]; ok {
		return true, nil
	}

	// Metadata cache did not have topic, try with fresh metadata
	lastMetaData, err = GetMetaData()
	if err != nil {
		zap.S().Errorf("Failed to get Kafka metadata: %s", err)
		return false, err
	}

	if _, ok := lastMetaData.Topics[kafkaTopicName]; ok {
		return true, nil
	}

	return
}

func GetMetaData() (metadata *kafka.Metadata, err error) {
	metadata, err = KafkaAdminClient.GetMetadata(nil, true, 10*1000)
	return
}

//goland:noinspection GoVetLostCancel
func CreateTopicIfNotExists(kafkaTopicName string) (err error) {
	exists, err := TopicExists(kafkaTopicName)
	if err != nil {
		zap.S().Debugf("Failed to check if topic %s exists: %s", kafkaTopicName, err)
		return err
	}
	if exists {
		return
	}

	topicSpecification := kafka.TopicSpecification{
		Topic:         kafkaTopicName,
		NumPartitions: 6,
	}
	var maxExecutionTime = time.Duration(5) * time.Second
	d := time.Now().Add(maxExecutionTime)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()
	topics, err := KafkaAdminClient.CreateTopics(ctx, []kafka.TopicSpecification{topicSpecification})
	if err != nil || len(topics) != 1 {
		zap.S().Errorf("Failed to create Topic %s : %s", kafkaTopicName, err)
		return
	}

	// send a message to the "umh.kafka.topic.created" topic with the new topic name
	// to trigger the subscriptions of the other consumers to the newly created topic
	payload := make(map[string]string)
	payload["topic"] = kafkaTopicName
	jsonString, err := json.Marshal(payload)
	if err != nil {
		zap.S().Errorf("Failed to marshal payload: %s", err)
		return
	}
	err = KafkaProducer.Produce(
		&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &probeTopicName,
				Partition: kafka.PartitionAny,
			},
			Value: jsonString,
		}, nil)
	if err != nil {
		return err
	}

	select {
	case <-time.After(maxExecutionTime):
		zap.S().Errorf("Topic creation deadline reached")
		return
	case <-ctx.Done():
		err = ctx.Err()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			zap.S().Errorf("Failed to await deadline: %s", err)
			return
		}
	}
	return
}

func MqttTopicToKafka(MqttTopicName string) (validTopic bool, KafkaTopicName string) {
	MqttTopicName = strings.TrimSpace(MqttTopicName)
	MqttTopicName = strings.ReplaceAll(MqttTopicName, "/", ".")
	MqttTopicName = strings.ReplaceAll(MqttTopicName, " ", "")
	if !IsKafkaTopicValid(MqttTopicName) {
		zap.S().Errorf("Topic is not valid: (%s), does not match %s", MqttTopicName, KafkaUMHTopicRegex)
		return false, ""
	}
	if len(strings.Split(MqttTopicName, ".")) >= 10 {
		zap.S().Errorf("Illegal Topic name: (%s) (max topic depth)", MqttTopicName)
		return false, ""
	}
	if len(MqttTopicName) >= 200 {
		zap.S().Errorf("Illegal Topic name: (%s) (max topic length)", MqttTopicName)
		return false, ""
	}
	return true, MqttTopicName
}
func KafkaTopicToMqtt(KafkaTopicName string) (validTopic bool, MqttTopicName string) {
	if strings.Contains(KafkaTopicName, "/") {
		zap.S().Errorf("Illegal Topic name: %s", KafkaTopicName)
		return false, ""
	}
	KafkaTopicName = strings.TrimSpace(KafkaTopicName)
	KafkaTopicName = strings.ReplaceAll(KafkaTopicName, " ", "")

	if len(strings.Split(KafkaTopicName, ".")) >= 10 {
		zap.S().Errorf("Illegal Topic name: %s (max topic depth)", KafkaTopicName)
		return false, ""
	}

	KafkaTopicName = strings.ReplaceAll(KafkaTopicName, ".", "/")
	if len(KafkaTopicName) >= 200 {
		zap.S().Errorf("Illegal Topic name: %s (max topic length)", KafkaTopicName)
		return false, ""
	}
	return true, KafkaTopicName
}
