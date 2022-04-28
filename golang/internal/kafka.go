//go:build kafka
// +build kafka

package internal

import (
	"context"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"regexp"
	"strings"
	"time"
)

var KafkaConsumer *kafka.Consumer
var KafkaProducer *kafka.Producer
var KafkaAdminClient *kafka.AdminClient

func SetupKafka(configMap kafka.ConfigMap) {

	var err error
	KafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	KafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		panic(err)
	}

	KafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	return
}

func CloseKafka() {
	err := KafkaConsumer.Close()
	if err != nil {
		panic("Failed do close KafkaConsumer client !")
	}

	KafkaProducer.Flush(100)
	KafkaProducer.Close()

	KafkaAdminClient.Close()
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
	metadata, err = KafkaAdminClient.GetMetadata(nil, true, 1*1000)
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
		NumPartitions: 6,
	}
	var maxExecutionTime = time.Duration(5) * time.Second
	d := time.Now().Add(maxExecutionTime)
	var ctx context.Context
	ctx, cancel = context.WithDeadline(context.Background(), d)
	topics, err := KafkaAdminClient.CreateTopics(ctx, []kafka.TopicSpecification{topicSpecification})
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

var validKafkaTopicRegex, _ = regexp.Compile(`^[a-zA-Z\d\._\-]+$`)

func MqttTopicToKafka(MqttTopicName string) (KafkaTopicName string) {

	MqttTopicName = strings.TrimSpace(MqttTopicName)
	MqttTopicName = strings.ReplaceAll(MqttTopicName, "/", ".")
	if !validKafkaTopicRegex.Match([]byte(MqttTopicName)) {
		zap.S().Errorf("Invalid MQTT->Kafka topic name: %s", MqttTopicName)
	}
	if !IsKafkaTopicValid(MqttTopicName) {
		zap.S().Errorf("Topic is not valid: %s, does not match %s", MqttTopicName, KafkaUMHTopicRegex)
	}
	return MqttTopicName
}
func KafkaTopicToMqtt(KafkaTopicName string) (MqttTopicName string) {
	if strings.Contains(KafkaTopicName, "/") {
		zap.S().Errorf("Illegal MQTT->Kafka Topic name: %s", KafkaTopicName)
	}
	KafkaTopicName = strings.TrimSpace(KafkaTopicName)
	KafkaTopicName = strings.TrimSpace(KafkaTopicName)
	return strings.ReplaceAll(KafkaTopicName, ".", "/")
}
