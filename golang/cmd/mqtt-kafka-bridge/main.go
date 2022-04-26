package main

import (
	"github.com/beeker1121/goque"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var mqttClient MQTT.Client
var mqttIncomingQueue *goque.Queue
var mqttOutGoingQueue *goque.Queue

var buildtime string

func main() {
	logger, _ := zap.NewProduction()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is mqtt-kafka-bridge build date: %s", buildtime)

	// Read environment variables for MQTT
	MQTTCertificateName := os.Getenv("MQTT_CERTIFICATE_NAME")
	MQTTBrokerURL := os.Getenv("MQTT_BROKER_URL")
	MQTTTopic := os.Getenv("MQTT_TOPIC")
	podName := os.Getenv("MY_POD_NAME")
	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	KafkaTopic := os.Getenv("KAFKA_LISTEN_TOPIC")
	KafkaBaseTopic := os.Getenv("KAFKA_BASE_TOPIC")

	zap.S().Debugf("Setting up memorycache")
	internal.InitMemcache()

	zap.S().Debugf("Setting up Queues")
	var err error
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

	// Prometheus
	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go http.ListenAndServe("0.0.0.0:8086", health)

	zap.S().Debugf("Setting up MQTT")
	//mqttClient = setupMQTT(MQTTCertificateName, MQTTBrokerURL, MQTTTopic, MQTTBrokerSSLEnabled, mqttIncomingQueue)
	SetupMQTT(MQTTCertificateName, MQTTBrokerURL, MQTTTopic, health, podName, mqttIncomingQueue)

	zap.S().Debugf("Setting up Kafka")
	internal.SetupKafka(kafka.ConfigMap{
		"bootstrap.servers": KafkaBoostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "mqtt-kafka-bridge",
	})
	err = internal.CreateTopicIfNotExists(KafkaBaseTopic)
	if err != nil {
		panic(err)
	}

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

	internal.CloseKafka()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

var validKafkaTopicRegex, _ = regexp.Compile(`^[a-zA-Z\d\._\-]+$`)

func MqttTopicToKafka(MqttTopicName string) (KafkaTopicName string) {

	MqttTopicName = strings.TrimSpace(MqttTopicName)
	MqttTopicName = strings.ReplaceAll(MqttTopicName, "/", ".")
	if !validKafkaTopicRegex.Match([]byte(MqttTopicName)) {
		zap.S().Errorf("Invalid MQTT->Kafka topic name: %s", MqttTopicName)
	}
	return MqttTopicName
}
func KafkaTopicToMqtt(KafkaTopicName string) (MqttTopicName string) {
	if strings.Contains(KafkaTopicName, "/") {
		zap.S().Errorf("Illegal MQTT->Kafka Topic name: %s", KafkaTopicName)
	}
	KafkaTopicName = strings.TrimSpace(KafkaTopicName)
	return strings.ReplaceAll(KafkaTopicName, ".", "/")
}
