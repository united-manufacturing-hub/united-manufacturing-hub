package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"
	"time"
)

func setupKafka(boostrapServer string) (consumer *kafka.Consumer, producer *kafka.Producer) {
	configMap := kafka.ConfigMap{
		"bootstrap.servers": boostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "mqtt-kafka-bridge",
	}

	var err error
	consumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	producer, err = kafka.NewProducer(&configMap)
	if err != nil {
		panic(err)
	}
	return
}

func processKafkaQueue(topic string, bucketName string) {
	err := kafkaConsumerClient.Subscribe(topic, nil)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for !ShuttingDown {
		msg, err := kafkaConsumerClient.ReadMessage(5) //No infinitive timeout to be able to cleanly shut down
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			} else {
				zap.S().Errorf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		rawImage, err := UnmarshalRawImage(msg.Value)
		if err != nil {
			zap.S().Warnf("Invalid rawImage: %s", err)
			continue
		}

		uid := rawImage.ImageID
		imgBytes, err := base64.StdEncoding.DecodeString(rawImage.ImageBytes)

		if err != nil {
			zap.S().Warnf("Image decoding failed: %s", err)
		}

		r := bytes.NewReader(imgBytes)
		_, err = minioClient.PutObject(ctx, bucketName, uid, r, -1, minio.PutObjectOptions{})
		if err != nil {
			zap.S().Warnf("Failed to put item into blob-storage: %s", err)
			kafkaProducerClient.Produce(&kafka.Message{
				TopicPartition: kafka.TopicPartition{
					Topic:     msg.TopicPartition.Topic,
					Partition: kafka.PartitionAny,
				},
				Value: msg.Value,
			}, nil)
			time.Sleep(1 * time.Second)
			continue
		}
	}
}
