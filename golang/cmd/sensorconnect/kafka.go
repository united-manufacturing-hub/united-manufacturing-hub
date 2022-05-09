package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"os"
)

// SendKafkaMessage tries to send a message via kafka
func SendKafkaMessage(kafkaTopicName string, message []byte) {
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

	err := internal.CreateTopicIfNotExists(kafkaTopicName)
	if err != nil {
		zap.S().Errorf("Failed to create topic %s", err)
		panic("Failed to create topic, restarting")
	}

	err = kafkaProducerClient.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &kafkaTopicName,
			Partition: kafka.PartitionAny,
		},
		Value: message,
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
	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
		securityProtocol = "ssl"

		_, err := os.Open("/SSL_certs/tls.key")
		if err != nil {
			panic("SSL key file not found")
		}
		_, err = os.Open("/SSL_certs/tls.crt")
		if err != nil {
			panic("SSL cert file not found")
		}
		_, err = os.Open("/SSL_certs/ca.crt")
		if err != nil {
			panic("SSL CA cert file not found")
		}
	}
	configMap := kafka.ConfigMap{
		"security.protocol":        securityProtocol,
		"ssl.key.location":         "/SSL_certs/tls.key",
		"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
		"ssl.certificate.location": "/SSL_certs/tls.crt",
		"ssl.ca.location":          "/SSL_certs/ca.crt",
		"bootstrap.servers":        boostrapServer,
		"group.id":                 "sensorconnect",
	}
	producer, err := kafka.NewProducer(&configMap)

	if err != nil {
		panic(err)
	}

	adminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	internal.KafkaProducer = producer
	internal.KafkaAdminClient = adminClient

	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					zap.S().Errorf("Delivery failed: %v (%s)", ev.TopicPartition, ev.TopicPartition.Error)
				}
			}
		}
	}()

	return
}
