package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var mqttClient MQTT.Client
var kafkaClient *kafka.Producer

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

	zap.S().Debugf("Setting up Queues")
	mqttIncomingQueue, err := setupQueue("incoming")
	if err != nil {
		zap.S().Errorf("Error setting up incoming queue", err)
		return
	}
	defer closeQueue(mqttIncomingQueue)

	mqttOutGoingQueue, err := setupQueue("outgoing")
	if err != nil {
		zap.S().Errorf("Error setting up outgoing queue", err)
		return
	}
	defer closeQueue(mqttOutGoingQueue)

	zap.S().Debugf("Setting up MQTT")
	mqttClient = setupMQTT(MQTTCertificateName, MQTTBrokerURL, MQTTTopic, MQTTBrokerSSLEnabled, mqttIncomingQueue)

	zap.S().Debugf("Setting up Kafka")
	kafkaClient = setupKafka(KafkaBoostrapServer)

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

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")

	mqttClient.Disconnect(1000)
	kafkaClient.Flush(1000)
	kafkaClient.Close()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
