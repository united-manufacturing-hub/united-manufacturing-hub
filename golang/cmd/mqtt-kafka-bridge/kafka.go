package main

import (
	"context"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"time"
)

func setupKafka(boostrapServer string) (producer *kafka.Producer) {
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": boostrapServer,
		"security.protocol": "plaintext",
	})

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
					zap.S().Infof("Delivered message to %v", ev.TopicPartition)
				}
			}
		}
	}()
	return
}

func processIncomingMessages() {
	var cancel context.CancelFunc
	var client *kafka.AdminClient
	var err error

	for !ShuttingDown {
		if client == nil {
			client, err = kafka.NewAdminClientFromProducer(kafkaClient)
			if err != nil {
				zap.S().Errorf("Failed to create Kafka admin client : %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

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
		var kafkaTopicName string
		kafkaTopicName = MqttTopicToKafka(object.Topic)
		topicSpecification := kafka.TopicSpecification{
			Topic:         kafkaTopicName,
			NumPartitions: 1,
		}

		//Cancel if topic creation takes to long
		var maxExecutionTime = time.Duration(5) * time.Second
		d := time.Now().Add(maxExecutionTime)
		var ctx context.Context
		ctx, cancel = context.WithDeadline(context.Background(), d)
		topics, err := client.CreateTopics(ctx, []kafka.TopicSpecification{topicSpecification})
		if err != nil || len(topics) != 1 {
			zap.S().Errorf("Failed to create Topic %s : %s", kafkaTopicName, err)
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			continue
		}

		select {
		case <-time.After(maxExecutionTime):
			zap.S().Errorf("Topic creation deadline reached")
			storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
			cancel()
			continue
		case <-ctx.Done():
			err = ctx.Err()
			if err != nil && err != context.DeadlineExceeded {
				zap.S().Errorf("Failed to await deadline: %s", err)
				storeMessageIntoQueue(object.Topic, object.Message, mqttIncomingQueue)
				cancel()
				continue
			}
		}

		zap.S().Debugf("Sending with Topic: %s", kafkaTopicName)
		err = kafkaClient.Produce(&kafka.Message{
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
	if client != nil {
		client.Close()
	}
	if cancel != nil {
		cancel()
	}
}
