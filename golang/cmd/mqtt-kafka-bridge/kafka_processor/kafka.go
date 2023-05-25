package kafka_processor

import (
	"github.com/Shopify/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/mqtt-kafka-bridge/message"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"regexp"
	"strings"
	"time"
)

var client *kafka.Client

func Init(kafkaToMqttChan chan kafka.Message, sChan chan bool) {
	if client != nil {
		return
	}
	KafkaBootstrapServer, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	KafkaTopic, err := env.GetAsString("KAFKA_LISTEN_TOPIC", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	useSsl, err := env.GetAsBool("KAFKA_USE_SSL", false, false)
	if err != nil {
		zap.S().Error(err)
	}

	compile, err := regexp.Compile(KafkaTopic)
	if err != nil {
		zap.S().Fatalf("Error compiling regex: %v", err)
	}

	client, err = kafka.NewKafkaClient(&kafka.NewClientOptions{
		Brokers: []string{
			KafkaBootstrapServer,
		},
		ConsumerName:      "mqtt-kafka-bridge",
		ListenTopicRegex:  compile,
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         useSsl,
		StartOffset:       sarama.OffsetOldest,
	})
	if err != nil {
		zap.S().Fatalf("Error creating kafka client: %v", err)
		return
	}
	go processIncomingMessage(kafkaToMqttChan)
}

func processIncomingMessage(kafkaToMqttChan chan kafka.Message) {
	for {
		msg := <-client.GetMessages()
		kafkaToMqttChan <- kafka.Message{
			Topic:  msg.Topic,
			Value:  msg.Value,
			Header: msg.Header,
			Key:    msg.Key,
		}

	}
}

func Shutdown() {
	zap.S().Info("Shutting down kafka client")
	err := client.Close()
	if err != nil {
		zap.S().Fatalf("Error closing kafka client: %v", err)
	}
	zap.S().Info("Kafka client shut down")
}

func Start(mqttToKafkaChan chan kafka.Message) {
	KafkaSenderThreads, err := env.GetAsInt("KAFKA_SENDER_THREADS", false, 1)
	if err != nil {
		zap.S().Error(err)
	}
	if KafkaSenderThreads < 1 {
		zap.S().Fatal("KAFKA_SENDER_THREADS must be at least 1")
	}
	for i := 0; i < KafkaSenderThreads; i++ {
		go start(mqttToKafkaChan)
	}
}

func start(mqttToKafkaChan chan kafka.Message) {
	for {
		msg := <-mqttToKafkaChan

		msg.Topic = strings.ReplaceAll(msg.Topic, "$share/MQTT_KAFKA_BRIDGE/", "")
		if !message.IsValidMQTTMessage(msg.Topic, msg.Value) {
			continue
		}
		// Change MQTT to Kafka topic format
		msg.Topic = strings.ReplaceAll(msg.Topic, "/", ".")

		internal.AddSXOrigin(&msg)
		var err error
		err = internal.AddSXTrace(&msg)
		if err != nil {
			zap.S().Fatalf("Failed to marshal trace")
			continue
		}

		err = client.EnqueueMessage(msg)
		for err != nil {
			time.Sleep(10 * time.Millisecond)
			err = client.EnqueueMessage(msg)
		}
	}
}

func GetStats() (sent, received, sentBytesA, recvBytesA uint64) {
	return kafka.GetKafkaStats()
}
