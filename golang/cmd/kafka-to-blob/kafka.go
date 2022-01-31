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

func setupKafka(boostrapServer string) (consumer *kafka.Consumer, producer *kafka.Producer, adminClient *kafka.AdminClient) {
	configMap := kafka.ConfigMap{
		"bootstrap.servers": boostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "kafka-to-blob",
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

	adminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	return
}

var lastMetaData *kafka.Metadata

func TopicExists(kafkaTopicName string) (exists bool, err error) {
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
		zap.S().Debugf("[CACHED] Topic %s exists", kafkaTopicName)
		return true, nil
	}

	//Metadata cache did not have topic, try with fresh metadata
	lastMetaData, err = GetMetaData()
	if err != nil {
		return false, err
	}

	if _, ok := lastMetaData.Topics[kafkaTopicName]; ok {
		zap.S().Debugf("[CACHED] Topic %s exists", kafkaTopicName)
		return true, nil
	}

	return
}

func GetMetaData() (metadata *kafka.Metadata, err error) {
	metadata, err = kafkaAdminClient.GetMetadata(nil, true, 1*1000)
	return
}

//goland:noinspection GoVetLostCancel
func CreateTopicIfNotExists(kafkaTopicName string) (err error) {
	exists, err := TopicExists(kafkaTopicName)
	if err != nil {
		return err
	}
	if exists {
		return
	}

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
	var maxExecutionTime = time.Duration(5) * time.Second
	d := time.Now().Add(maxExecutionTime)
	var ctx context.Context
	ctx, cancel = context.WithDeadline(context.Background(), d)
	topics, err := kafkaAdminClient.CreateTopics(ctx, []kafka.TopicSpecification{topicSpecification})
	if err != nil || len(topics) != 1 {
		zap.S().Errorf("Failed to create Topic %s : %s", kafkaTopicName, err)
		return
	}

	select {
	case <-time.After(maxExecutionTime):
		zap.S().Errorf("Topic creation deadline reached")
		return
	case <-ctx.Done():
		err = ctx.Err()
		if err != nil && err != context.DeadlineExceeded {
			zap.S().Errorf("Failed to await deadline: %s", err)
			return
		}
	}
	return
}

func processKafkaQueue(topic string, bucketName string) {
	err := kafkaConsumerClient.Subscribe(topic, nil)
	if err != nil {
		panic(err)
	}

	for !ShuttingDown {
		if minioClient.IsOffline() {
			zap.S().Warnf("Minio is down")
			time.Sleep(10 * time.Second)
			continue
		}
		var msg *kafka.Message
		msg, err = kafkaConsumerClient.ReadMessage(5) //No infinitive timeout to be able to cleanly shut down
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			} else if err.(kafka.Error).Code() == kafka.ErrUnknownTopicOrPart {
				time.Sleep(5 * time.Second)
				continue
			} else {
				zap.S().Warnf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}

		}

		var rawImage RawImage
		rawImage, err = UnmarshalRawImage(msg.Value)
		if err != nil {
			zap.S().Warnf("Invalid rawImage: %s", err)
			continue
		}

		uid := rawImage.ImageID
		var imgBytes []byte
		imgBytes, err = base64.StdEncoding.DecodeString(rawImage.ImageBytes)

		if err != nil {
			zap.S().Warnf("Image decoding failed: %s", err)
		}

		go pushToMinio(imgBytes, uid, bucketName, msg)
	}
}

func pushToMinio(imgBytes []byte, uid string, bucketName string, msg *kafka.Message) {
	ctx := context.Background()

	r := bytes.NewReader(imgBytes)
	var upinfo minio.UploadInfo
	var err error
	start := time.Now()
	upinfo, err = minioClient.PutObject(ctx, bucketName, uid, r, -1, minio.PutObjectOptions{})

	if err != nil {
		zap.S().Warnf("Failed to put item into blob-storage: %s", err)
		kafkaProducerClient.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     msg.TopicPartition.Topic,
				Partition: kafka.PartitionAny,
			},
			Value: msg.Value,
		}, nil)
		return
	}

	elapsed := time.Since(start)
	zap.S().Debugf("Commited to blob storage in %s", elapsed)
	zap.S().Debugf("%s/%s", upinfo.Bucket, upinfo.Key)
}
