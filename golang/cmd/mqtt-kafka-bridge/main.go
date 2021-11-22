package main

import (
	"github.com/beeker1121/goque"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var mqttClient MQTT.Client
var kafkaProducerClient *kafka.Producer
var kafkaAdminClient *kafka.AdminClient
var kafkaConsumerClient *kafka.Consumer
var mqttIncomingQueue *goque.Queue
var mqttOutGoingQueue *goque.Queue

var buildtime string

func main() {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is mqtt-kafka-bridge build date: %s", buildtime)

	// Read environment variables for MQTT
	MQTTCertificateName := os.Getenv("MQTT_CERTIFICATE_NAME")
	MQTTBrokerURL := os.Getenv("MQTT_BROKER_URL")
	MQTTTopic := os.Getenv("MQTT_TOPIC")
	MQTTBrokerSSLEnabled, err := strconv.ParseBool(os.Getenv("MQTT_BROKER_SSL_ENABLED"))
	if err != nil {
		zap.S().Errorf("Error parsing bool from environment variable", err)
		return
	}
	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	KafkaTopic := os.Getenv("KAFKA_LISTEN_TOPIC")

	// Redis cache
	redisURI := os.Getenv("REDIS_URI")
	redisURI2 := os.Getenv("REDIS_URI2")
	redisURI3 := os.Getenv("REDIS_URI3")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	dryRun := os.Getenv("DRY_RUN")

	redisDB := 0 // default database
	zap.S().Debugf("Setting up redis")
	internal.InitCache(redisURI, redisURI2, redisURI3, redisPassword, redisDB, dryRun)

	zap.S().Debugf("Setting up Queues")
	mqttIncomingQueue, err = setupQueue("incoming")
	if err != nil {
		zap.S().Errorf("Error setting up incoming queue", err)
		return
	}
	defer closeQueue(mqttIncomingQueue)

	mqttOutGoingQueue, err = setupQueue("outgoing")
	if err != nil {
		zap.S().Errorf("Error setting up outgoing queue", err)
		return
	}
	defer closeQueue(mqttOutGoingQueue)

	zap.S().Debugf("Setting up MQTT")
	mqttClient = setupMQTT(MQTTCertificateName, MQTTBrokerURL, MQTTTopic, MQTTBrokerSSLEnabled, mqttIncomingQueue)

	zap.S().Debugf("Setting up Kafka")
	kafkaProducerClient, kafkaAdminClient, kafkaConsumerClient = setupKafka(KafkaBoostrapServer)

	zap.S().Debugf("Start Queue processors")
	go processIncomingMessages()
	go processOutgoingMessages()
	go kafkaToQueue(KafkaTopic)

	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	go func() {
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.
		//
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Recieved SIGTERM", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {} // block forever
}

var ShuttingDown bool

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	ShuttingDown = true
	mqttClient.Disconnect(1000)

	if kafkaProducerClient != nil {
		kafkaProducerClient.Flush(1000)
		kafkaProducerClient.Close()
	}
	if kafkaAdminClient != nil {
		kafkaAdminClient.Close()
	}
	if kafkaConsumerClient != nil {
		kafkaConsumerClient.Close()
	}

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

func MqttTopicToKafka(MqttTopicName string) (KafkaTopicName string) {
	if strings.Contains(MqttTopicName, ".") {
		zap.S().Errorf("Illegal MQTT Topic name received: %s", MqttTopicName)
	}
	return strings.ReplaceAll(MqttTopicName, "/", ".")
}
func KafkaTopicToMqtt(KafkaTopicName string) (MqttTopicName string) {
	if strings.Contains(KafkaTopicName, "/") {
		zap.S().Errorf("Illegal Kafka Topic name received: %s", KafkaTopicName)
	}
	return strings.ReplaceAll(KafkaTopicName, ".", "/")
}
