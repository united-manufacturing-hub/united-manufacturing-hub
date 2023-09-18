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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
)

type kafkaClient struct {
	client *kafka.Client
}

func newKafkaClient(broker, topic, serialNumber string, partitions, replicationFactor int) (kc *kafkaClient, err error) {
	kc = &kafkaClient{}
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
	consumerGroupId := "data-bridge-" + hex.EncodeToString(hasher.Sum(nil))

	options := &kafka.NewClientOptions{
		Brokers: []string{
			broker,
		},
		ConsumerGroupId:   consumerGroupId,
		ListenTopicRegex:  topicsRegex,
		Partitions:        int32(partitions),
		ReplicationFactor: int16(replicationFactor),
		StartOffset:       sarama.OffsetOldest,
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
func (k *kafkaClient) startProducing(msgChan chan kafka.Message, split int) {
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
		}
	}()
}

// startConsuming starts to read incoming messages from kafka and sends them to the msgChan
func (k *kafkaClient) startConsuming(msgChan chan kafka.Message) {
	go func() {
		for {
			msg := <-k.client.GetMessages()
			msgChan <- kafka.Message{
				Topic:  msg.Topic,
				Value:  msg.Value,
				Header: msg.Header,
				Key:    msg.Key,
			}
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
