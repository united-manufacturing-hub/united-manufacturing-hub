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
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
)

type kafkaClient struct {
	client *kafka.Client
}

func newKafkaClient(broker, topic, serialNumber string) (kc *kafkaClient, err error) {
	kc = &kafkaClient{}

	partitions, err := env.GetAsInt("PARTITIONS", false, 6)
	if err != nil {
		zap.S().Error(err)
	}
	if partitions < 1 {
		zap.S().Fatalf("PARTITIONS must be at least 1. got: %d", partitions)
	}
	replicationFactor, err := env.GetAsInt("REPLICATION_FACTOR", false, 1)
	if err != nil {
		zap.S().Error(err)
	}
	if replicationFactor%2 == 0 {
		zap.S().Fatalf("REPLICATION_FACTOR must be odd. got: %d", replicationFactor)
	}

	topic, err = toKafkaTopic(topic)
	if err != nil {
		return nil, err
	}
	topicsRegex, err := regexp.Compile(topic)
	if err != nil {
		zap.S().Fatalf("error compiling regex: %v", err)
	}

	hasher := sha3.New256()
	hasher.Write([]byte(serialNumber))
	consumerGroupId := "databridge-" + hex.EncodeToString(hasher.Sum(nil))

	podName, err := env.GetAsString("POD_NAME", true, "")
	if err != nil {
		zap.S().Fatalf("error getting pod name: %v", err)
	}

	options := &kafka.NewClientOptions{
		Brokers: []string{
			broker,
		},
		ConsumerGroupId:   consumerGroupId,
		ListenTopicRegex:  topicsRegex,
		Partitions:        int32(partitions),
		ReplicationFactor: int16(replicationFactor),
		StartOffset:       sarama.OffsetOldest,
		AutoMark:          false,
		ClientID:          podName,
	}

	kc.client, err = kafka.NewKafkaClient(options)
	return
}

func (k *kafkaClient) getProducerStats() (sent uint64) {
	sent, _, _, _ = kafka.GetKafkaStats()
	return
}

func (k *kafkaClient) getConsumerStats() (received uint64) {
	_, received, _, _ = kafka.GetKafkaStats()
	return
}

// startProducing starts to read incoming messages from msgChan, transforms them
// into valid kafka messagges, does the splitting and sends them to kafka
func (k *kafkaClient) startProducing(msgChan chan kafka.Message, commitChan chan *kafka.Message, split int) {
	go func() {
		for {
			msg := <-msgChan

			var err error
			msg.Topic, err = toKafkaTopic(msg.Topic)
			if err != nil {
				zap.S().Warnf("skipping message: %s", err)
				continue
			}

			if !isValidKafkaMessage(msg) {
				zap.S().Warnf("skipping message: %s", msg.Topic)
				continue
			}

			msg = splitMessage(msg, split)

			internal.AddSXOrigin(&msg)
			err = internal.AddSXTrace(&msg)
			if err != nil {
				zap.S().Fatalf("failed to marshal trace")
				continue
			}

			err = k.client.EnqueueMessage(msg)
			for err != nil {
				time.Sleep(10 * time.Millisecond)
				err = k.client.EnqueueMessage(msg)
			}

			commitChan <- &msg
		}
	}()
}

// startConsuming starts to read incoming messages from kafka and sends them to the msgChan
func (k *kafkaClient) startConsuming(msgChan chan kafka.Message, commitChan chan *kafka.Message) {
	go func() {
		for msg := range k.client.GetMessages() {
			msgChan <- msg
		}
	}()
	go func() {
		for msg := range commitChan {
			k.client.MarkMessage(msg)
		}
	}()
}

func (k *kafkaClient) shutdown() error {
	zap.S().Info("shutting down kafka client")
	return k.client.Close()
}

// splitMessage splits the topic of msg into two parts, the first part will be
// the topic of splittedMsg, the second part will be the key of splittedMsg.
//
// If the topic of msg has less than split parts, splittedMsg will have the same
// topic and key as msg.
// The key of msg will always be appended to the end of the key of splittedMsg.
func splitMessage(msg kafka.Message, split int) (splittedMsg kafka.Message) {
	parts := strings.Split(msg.Topic, ".")

	if len(parts) < split {
		splittedMsg.Topic = msg.Topic
		splittedMsg.Key = msg.Key
	} else {
		splittedMsg.Topic = strings.Join(parts[:split], ".")
		// append existing key to the end of the new key, in order to account
		// for messages that have already been split
		splittedMsg.Key = append([]byte(strings.Join(parts[split:], ".")), msg.Key...)
	}
	splittedMsg.Value = msg.Value
	splittedMsg.Header = msg.Header

	return splittedMsg
}

func isValidKafkaMessage(message kafka.Message) bool {
	if !json.Valid(message.Value) {
		zap.S().Warnf("not a valid json in message: %s", message.Topic)
		return false
	}

	if internal.IsSameOrigin(&message) {
		zap.S().Warnf("message from same origin: %s", message.Topic)
		return false
	}

	if internal.IsInTrace(&message) {
		zap.S().Warnf("message in trace: %s", message.Topic)
		return false
	}

	return true
}

func isValidKafkaTopic(topic string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9\._-]+(\.\*)?$`).MatchString(topic)
}

func toKafkaTopic(topic string) (string, error) {
	if strings.HasPrefix(topic, "$share") {
		topic = string(regexp.MustCompile(`\$share\/DATA_BRIDGE_(.*?)\/`).ReplaceAll([]byte(topic), []byte("")))
	}
	if isValidMqttTopic(topic) {
		topic = strings.ReplaceAll(topic, "/", ".")
		topic = strings.ReplaceAll(topic, "#", ".*")
		return topic, nil
	} else if isValidKafkaTopic(topic) {
		return topic, nil
	}

	return "", fmt.Errorf("invalid topic: %s", topic)
}
