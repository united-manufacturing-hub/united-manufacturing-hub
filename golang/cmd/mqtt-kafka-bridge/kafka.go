package main

import (
	"context"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"time"
)

func setupKafka(boostrapServer string) (producer *kafka.Producer, adminClient *kafka.AdminClient, consumer *kafka.Consumer) {
	configMap := kafka.ConfigMap{
		"bootstrap.servers": boostrapServer,
		"security.protocol": "plaintext",
	}
	producer, err := kafka.NewProducer(&configMap)

	if err != nil {
		panic(err)
	}

	adminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	/*
		consumer, err = kafka.NewConsumer(&configMap)
		if err != nil {
			panic(err)
		}

	*/

	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					zap.S().Errorf("Delivery failed: %v (%s)", ev.TopicPartition, ev.TopicPartition.Error)
				} else {
					zap.S().Infof("Delivered message to %v", ev.TopicPartition)
				}
			}
		}
	}()
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
func CreateTopicIfNotExists(mqttTopicName string) (err error) {
	kafkaTopicName := MqttTopicToKafka(mqttTopicName)
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

func processIncomingMessages() {
	var err error

	for !ShuttingDown {
		if mqttIncomingQueue.Length() == 0 {
			//Skip if empty
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var object queueObject
		object, err = retrieveMessageFromQueue(mqttIncomingQueue)
		if err != nil {
			zap.S().Errorf("Failed to dequeue message: %s", err)
			time.Sleep(1 * time.Second)
			continue
		}

		//Setup Topic if not exist
		err = CreateTopicIfNotExists(object.Topic)
		if err != nil {
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			continue
		}

		kafkaTopicName := MqttTopicToKafka(object.Topic)
		zap.S().Debugf("Sending with Topic: %s", kafkaTopicName)
		err = kafkaProducerClient.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &kafkaTopicName,
				Partition: kafka.PartitionAny,
			},
			Value: object.Message,
		}, nil)
		if err != nil {
			zap.S().Errorf("Failed to send Kafka message: %s", err)
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			continue
		}
	}
}
