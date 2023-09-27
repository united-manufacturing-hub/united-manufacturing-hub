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
	"context"
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
	marked             atomic.Uint64
	lossInvalidMessage atomic.Uint64
	skipped            atomic.Uint64
	consumer           *consumer.Consumer
	producer           *producer.Producer
	split              int
}

func newKafkaClient(broker, topic, serialNumber string, split int) (kc *kafkaClient, err error) {
	kc = &kafkaClient{
		split: split,
	}

	podName, err := env.GetAsString("POD_NAME", true, "")
	if err != nil {
		return nil, err
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

	brokers = resolver(brokers)

	zap.S().Infof("connecting to kafka brokers: %s (topic: %s, consumer group: %s)", broker, topic, consumerGroupId)
	kc.consumer, err = consumer.NewConsumer(brokers, []string{topic}, consumerGroupId, podName)
	if err != nil {
		return nil, err
	}
	err = kc.consumer.Start(context.Background())
	if err != nil {
		return nil, err
	}
	kc.producer, err = producer.NewProducer(brokers)
	if err != nil {
		return nil, err
	}
	return
}

func (k *kafkaClient) getProducerStats() (sent uint64, invalidTopic uint64, invalidMessage uint64, skippedMessage uint64) {
	_, productionErrors := k.producer.GetProducedMessages()
	return k.marked.Load(), k.lossInvalidTopic.Load(), productionErrors + k.lossInvalidMessage.Load(), k.skipped.Load()
}

func (k *kafkaClient) getConsumerStats() (received uint64, invalidTopic uint64, invalidMessage uint64, skippedMessage uint64) {
	_, consumed := k.consumer.GetStats()
	return consumed, k.lossInvalidTopic.Load(), k.lossInvalidMessage.Load(), k.skipped.Load()
}

// startProducing starts to read incoming messages from msgChan, transforms them
// into valid kafka messages, does the splitting and sends them to kafka
func (k *kafkaClient) startProducing(toProduceMessageChannel chan *shared.KafkaMessage, bridgedMessagesToCommitChannel chan *shared.KafkaMessage) {
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

			msg = splitMessage(msg, k.split)

			k.producer.SendMessage(msg)

			bridgedMessagesToCommitChannel <- msg
		}
	}()
}

// startConsuming starts to read incoming messages from kafka and sends them to the msgChan
func (k *kafkaClient) startConsuming(receivedMessageChannel chan *shared.KafkaMessage, bridgedMessagesToCommitChannel chan *shared.KafkaMessage) {
	go func() {
		for {
			select {
			case msg := <-k.consumer.GetMessages():
				receivedMessageChannel <- msg
			}
		}
	}()
	go func() {
		for {
			select {
			case msg := <-bridgedMessagesToCommitChannel:
				msg.Topic = strings.ReplaceAll(msg.Topic, "/", ".")
				msg = splitMessage(msg, k.split)
				k.consumer.MarkMessage(msg)
				k.marked.Add(1)
			}
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
	splittedMsg = &shared.KafkaMessage{
		Offset:    msg.Offset,
		Partition: msg.Partition,
	}

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
