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
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/consumer"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/producer"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
	"regexp"
	"strings"
	"sync/atomic"
)

type kafkaClient struct {
	lossInvalidTopic   atomic.Uint64
	lossInvalidMessage atomic.Uint64
	skipped            atomic.Uint64
	consumer           *consumer.Consumer
	producer           *producer.Producer
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

	hasher := sha3.New256()
	hasher.Write([]byte(serialNumber))
	consumerGroupId := "databridge-" + hex.EncodeToString(hasher.Sum(nil))

	// split brokers by comma
	brokers := strings.Split(broker, ",")

	kc.consumer, err = consumer.NewConsumer(brokers, topic, consumerGroupId)

	return
}

func (k *kafkaClient) getProducerStats() (sent uint64, invalidTopic uint64, invalidMessage uint64, skippedMessage uint64) {
	produced, productionErrors := k.producer.GetProducedMessages()
	return produced, 0, productionErrors, 0
}

func (k *kafkaClient) getConsumerStats() (received uint64, invalidTopic uint64, invalidMessage uint64, skippedMessage uint64) {
	_, consumed := k.consumer.GetStats()
	return consumed, 0, 0, 0
}

// startProducing starts to read incoming messages from msgChan, transforms them
// into valid kafka messages, does the splitting and sends them to kafka
func (k *kafkaClient) startProducing(toProduceMessageChannel chan *shared.KafkaMessage, bridgedMessagesToCommitChannel chan *shared.KafkaMessage, split int) {
	go func() {
		for {
			msg := <-toProduceMessageChannel

			var err error
			msg.Topic, err = toKafkaTopic(msg.Topic)
			if err != nil {
				k.lossInvalidTopic.Add(1)
				zap.S().Warnf("skipping message (invalid topic): %s", err)
				continue
			}

			valid, jsonFailed := isValidKafkaMessage(msg)
			if !valid {
				if jsonFailed {
					k.lossInvalidMessage.Add(1)
				} else {
					k.skipped.Add(1)
				}
				continue
			}

			msg = splitMessage(msg, split)

			k.producer.SendMessage(msg)

			bridgedMessagesToCommitChannel <- msg
		}
	}()
}

// startConsuming starts to read incoming messages from kafka and sends them to the msgChan
func (k *kafkaClient) startConsuming(receivedMessageChannel chan *shared.KafkaMessage, bridgedMessagesToCommitChannel chan *shared.KafkaMessage) {
	go func() {
		for msg := range k.consumer.GetMessages() {
			receivedMessageChannel <- msg
		}
	}()
	go func() {
		for msg := range bridgedMessagesToCommitChannel {
			k.consumer.MarkMessage(msg)
		}
	}()
}

func (k *kafkaClient) shutdown() error {
	zap.S().Info("shutting down kafka clients")
	errProducer := k.producer.Close()
	errConsumer := k.consumer.Close()
	return errors.Join(errProducer, errConsumer)
}

// splitMessage splits the topic of msg into two parts, the first part will be
// the topic of splittedMsg, the second part will be the key of splittedMsg.
//
// If the topic of msg has less than split parts, splittedMsg will have the same
// topic and key as msg.
// The key of msg will always be appended to the end of the key of splittedMsg.
func splitMessage(msg *shared.KafkaMessage, split int) (splittedMsg *shared.KafkaMessage) {
	parts := strings.Split(msg.Topic, ".")
	splittedMsg = &shared.KafkaMessage{}

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
	splittedMsg.Headers = msg.Headers

	return splittedMsg
}

func isValidKafkaMessage(message *shared.KafkaMessage) (valid bool, jsonFailed bool) {
	if !json.Valid(message.Value) {
		zap.S().Warnf("not a valid json in message: %s", message.Topic)
		return false, true
	}

	if shared.IsSameOrigin(message) {
		zap.S().Warnf("message from same origin: %s", message.Topic)
		return false, false
	}

	if shared.IsInTrace(message) {
		zap.S().Warnf("message in trace: %s", message.Topic)
		return false, false
	}

	return true, false
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
