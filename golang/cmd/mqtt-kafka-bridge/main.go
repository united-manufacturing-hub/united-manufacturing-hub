package main

import (
	"github.com/beeker1121/goque"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var mqttClient MQTT.Client
var mqttIncomingQueue *goque.Queue
var mqttOutGoingQueue *goque.Queue

var buildtime string

func main() {
	var logLevel = os.Getenv("LOGGING_LEVEL")
	encoderConfig := ecszap.NewDefaultEncoderConfig()
	var core zapcore.Core
	switch logLevel {
	case "DEVELOPMENT":
		core = ecszap.NewCore(encoderConfig, os.Stdout, zap.DebugLevel)
	default:
		core = ecszap.NewCore(encoderConfig, os.Stdout, zap.InfoLevel)
	}
	logger := zap.New(core, zap.AddCaller())
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
		zap.S().Fatalf("Error setting up incoming queue", err)
		return
	}
	defer closeQueue(mqttIncomingQueue)

	mqttOutGoingQueue, err = setupQueue("outgoing")
	if err != nil {
		zap.S().Fatalf("Error setting up outgoing queue", err)
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
	internal.SetupKafka(kafka.ConfigMap{
		"security.protocol":        securityProtocol,
		"ssl.key.location":         "/SSL_certs/tls.key",
		"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
		"ssl.certificate.location": "/SSL_certs/tls.crt",
		"ssl.ca.location":          "/SSL_certs/ca.crt",
		"bootstrap.servers":        KafkaBoostrapServer,
		"group.id":                 "mqtt-kafka-bridge",
	})
	err = internal.CreateTopicIfNotExists(KafkaBaseTopic)
	if err != nil {
		panic(err)
	}

	zap.S().Debugf("Start Queue processors")
	go internal.StartEventHandler("MQTTKafkaBridge", internal.KafkaProducer.Events(), nil)
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

	go ReportStats()

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

func ReportStats() {
	lastConfirmed := 0.0
	for !ShuttingDown {
		zap.S().Infof("Reporting stats"+
			"| MQTT->Kafka queue lenght: %d"+
			"| Kafka->MQTT queue length: %d"+
			"| Produces Kafka messages: %f"+
			"| Produces Kafka messages/s: %f",
			mqttIncomingQueue.Length(),
			mqttOutGoingQueue.Length(),
			internal.KafkaConfirmed,
			(internal.KafkaConfirmed-lastConfirmed)/5,
		)
		lastConfirmed = internal.KafkaConfirmed
		time.Sleep(internal.FiveSeconds)
	}
}
