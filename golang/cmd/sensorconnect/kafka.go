package main

import (
	"context"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strings"
)

// SendKafkaMessage tries to send a message via kafka
func SendKafkaMessage(kafkaTopicName string, message []byte, key []byte) {
	if !useKafka {
		return
	}

	messageHash := xxh3.Hash(message)
	cacheKey := fmt.Sprintf("SendKafkaMessage%s%d", kafkaTopicName, messageHash)

	_, found := internal.GetMemcached(cacheKey)
	if found {
		zap.S().Debugf("Duplicate message for topic %s, you might want to increase LOWER_POLLING_TIME !", kafkaTopicName)
		return
	}

	err := CreateTopicIfNotExists(kafkaTopicName)
	if err != nil {
		zap.S().Errorf("Failed to create topic %s", err)
		panic("Failed to create topic, restarting")
		return
	}

	err = kafkaProducerClient.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &kafkaTopicName,
			Partition: kafka.PartitionAny,
		},
		Value: message,
		Key:   key,
	}, nil)
	if err != nil {
		zap.S().Errorf("Failed to send Kafka message: %s", err)
	} else {
		internal.SetMemcached(cacheKey, nil)
	}
}

// setupKafka sets up the connection to the kafka server
func setupKafka(boostrapServer string) (producer *kafka.Producer, adminClient *kafka.AdminClient) {
	if !useKafka {
		return
	}
	configMap := kafka.ConfigMap{
		"bootstrap.servers": boostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "sensorconnect",
		/*
			"linger.ms":                             10,
			"batch.size":                            16384,
			"max.in.flight.requests.per.connection": 60,*/
	}
	producer, err := kafka.NewProducer(&configMap)

	if err != nil {
		panic(err)
	}

	adminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					zap.S().Errorf("Delivery failed: %v (%s)", ev.TopicPartition, ev.TopicPartition.Error)
				} else {
					//zap.S().Debugf("Delivered message to %v", ev.TopicPartition)
				}
			}
		}
	}()

	return
}

var lastMetaData *kafka.Metadata

func TopicExists(kafkaTopicName string) (exists bool, err error) {
	if !useKafka {
		return
	}
	//Check if lastMetaData was initialized
	if lastMetaData == nil {
		// Get initial map of metadata
		lastMetaData, err = GetMetaData()
		if err != nil {
			return false, err
		}
	}

	//Check if current metadata cache has topic listed
	if _, ok := lastMetaData.Topics[kafkaTopicName]; ok {
		return true, nil
	}

	//Metadata cache did not have topic, try with fresh metadata
	lastMetaData, err = GetMetaData()
	if err != nil {
		return false, err
	}

	if _, ok := lastMetaData.Topics[kafkaTopicName]; ok {
		return true, nil
	}

	return
}

func GetMetaData() (metadata *kafka.Metadata, err error) {
	if !useKafka {
		return
	}
	metadata, err = kafkaAdminClient.GetMetadata(nil, true, 1*1000)
	return
}

//goland:noinspection GoVetLostCancel
func CreateTopicIfNotExists(kafkaTopicName string) (err error) {
	if !useKafka {
		return
	}
	exists, err := TopicExists(kafkaTopicName)
	if err != nil {
		return err
	}
	if exists {
		return
	}

	zap.S().Infof("Creating new Topic %s", kafkaTopicName)
	var cancel context.CancelFunc
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()
	topicSpecification := kafka.TopicSpecification{
		Topic:         kafkaTopicName,
		NumPartitions: 1,
	}
	topics, err := kafkaAdminClient.CreateTopics(context.Background(), []kafka.TopicSpecification{topicSpecification})
	if err != nil || len(topics) != 1 {
		zap.S().Errorf("Failed to create Topic %s : %s", kafkaTopicName, err)
		return
	}

	return
}

func MqttTopicToKafka(MqttTopicName string) (KafkaTopicName string) {
	if !useKafka {
		return
	}
	if strings.Contains(MqttTopicName, ".") {
		zap.S().Errorf("Illegal MQTT Topic name received: %s", MqttTopicName)
	}
	return strings.ReplaceAll(MqttTopicName, "/", ".")
}
