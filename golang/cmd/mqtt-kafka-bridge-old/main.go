// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/beeker1121/goque"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	lru "github.com/hashicorp/golang-lru"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"runtime/debug"
	"strconv"

	/* #nosec G108 -- Replace with https://github.com/felixge/fgtrace later*/
	_ "net/http/pprof"
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
	var err error
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err = logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	zap.S().Infof("This is mqtt-kafka-bridge build date: %s", buildtime)

	internal.Initfgtrace()

	// Read environment variables for MQTT
	MQTTCertificateName, MQTTCertificateNameEnvSet := os.LookupEnv("MQTT_CERTIFICATE_NAME")
	if !MQTTCertificateNameEnvSet {
		zap.S().Fatal("Mqtt certificate name (MQTT_CERTIFICATE_NAME) must be set")
	}
	MQTTBrokerURL, MQTTBrokerURLEnvSet := os.LookupEnv("MQTT_BROKER_URL")
	if !MQTTBrokerURLEnvSet {
		zap.S().Fatal("Mqtt broker url (MQTT_BROKER_URL) must be set")
	}
	MQTTTopic, MQTTTopicEnvSet := os.LookupEnv("MQTT_TOPIC")
	if !MQTTTopicEnvSet {
		zap.S().Fatal("Mqtt topic (MQTT_TOPIC) must be set")
	}
	podName, podNameEnvSet := os.LookupEnv("MY_POD_NAME")
	if !podNameEnvSet {
		zap.S().Fatal("Pod name (MY_POD_NAME) must be set")
	}
	mqttPassword, mqttPasswordEnvSet := os.LookupEnv("MQTT_PASSWORD")
	if !mqttPasswordEnvSet {
		zap.S().Fatal("Mqtt password (MQTT_PASSWORD) must be set")
	}
	// Read environment variables for Kafka
	KafkaBoostrapServer, KafkaBoostrapServerEnvSet := os.LookupEnv("KAFKA_BOOTSTRAP_SERVER")
	if !KafkaBoostrapServerEnvSet {
		zap.S().Fatal("Kafka Bootstrap server (KAFKA_BOOTSTRAP_SERVER) must be set")
	}
	KafkaTopic, KafkaTopicEnvSet := os.LookupEnv("KAFKA_LISTEN_TOPIC")
	if !KafkaTopicEnvSet {
		zap.S().Fatal("Kafka topic (KAFKA_LISTEN_TOPIC) must be set")
	}
	KafkaBaseTopic, KafkaBaseTopicEnvSet := os.LookupEnv("KAFKA_BASE_TOPIC")
	if !KafkaBaseTopicEnvSet {
		zap.S().Fatal("Kafka base topic (KAFKA_BASE_TOPIC) must be set")
	}
	KafkaAcceptNoOriginStr := os.Getenv("KAFKA_ACCEPT_NO_ORIGIN")
	KafkaAcceptNoOrigin, err = strconv.ParseBool(KafkaAcceptNoOriginStr)
	if err != nil {
		zap.S().Errorf("Error parsing KAFKA_ACCEPT_NO_ORIGIN: %v", err)
		KafkaAcceptNoOrigin = false
	}

	MQTTSenderThreads, err = strconv.Atoi(os.Getenv("MQTT_SENDER_THREADS"))
	if err != nil {
		MQTTSenderThreads = 4
	}

	KafkaSenderThreads, err = strconv.Atoi(os.Getenv("KAFKA_SENDER_THREADS"))
	if err != nil {
		KafkaSenderThreads = 4
	}

	zap.S().Debugf("Setting up LRU")
	RawLruSizeStr, foundRawLruSizeStr := os.LookupEnv("RAW_MESSAGE_LRU_SIZE")
	if !foundRawLruSizeStr {
		RawLruSizeStr = "100000"
	}
	var RawLruSize int
	RawLruSize, err = strconv.Atoi(RawLruSizeStr)
	if err != nil {
		zap.S().Fatalf("Error parsing RAW_MESSAGE_LRU_SIZE: %v", err)
	}
	RawMessageLRU, err = lru.NewARC(RawLruSize)

	LruSizeStr, foundLruSizeStr := os.LookupEnv("MESSAGE_LRU_SIZE")
	if !foundLruSizeStr {
		LruSizeStr = "100000"
	}
	var LruSize int
	LruSize, err = strconv.Atoi(LruSizeStr)
	if err != nil {
		zap.S().Fatalf("Error parsing MESSAGE_LRU_SIZE: %v", err)
	}
	MessageLRU, err = lru.NewARC(LruSize)
	if err != nil {
		zap.S().Fatalf("Error creating LRU: %v", err)
	}

	zap.S().Debugf("Setting up Queues")
	mqttIncomingQueue, err = setupQueue("incoming")
	if err != nil {
		zap.S().Fatalf("Error setting up incoming queue: %v", err)
		return
	}
	defer func(pq *goque.Queue) {
		err = closeQueue(pq)
		if err != nil {
			zap.S().Errorf("Error closing queue %v", err)
		}
	}(mqttIncomingQueue)

	mqttOutGoingQueue, err = setupQueue("outgoing")
	if err != nil {
		zap.S().Fatalf("Error setting up outgoing queue: %v", err)
		return
	}
	defer func(pq *goque.Queue) {
		err = closeQueue(pq)
		if err != nil {
			zap.S().Errorf("Error closing outgoing queue %v", err)
		}
	}(mqttOutGoingQueue)

	// Prometheus
	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err = http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Fatalf("Error starting healthcheck %v", err)
		}
	}()

	zap.S().Debugf("Setting up MQTT")
	SetupMQTT(MQTTCertificateName, MQTTBrokerURL, MQTTTopic, health, podName, mqttIncomingQueue, mqttPassword)

	zap.S().Debugf("Setting up Kafka")
	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
		securityProtocol = "ssl"

		_, err = os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatalf("Error opening kafka key file: %v", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Error opening kafka cert %v", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Error opening ca.crt: %v", err)
		}
	}
	internal.SetupKafka(
		kafka.ConfigMap{
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"bootstrap.servers":        KafkaBoostrapServer,
			"group.id":                 "mqtt-kafka-bridge",
			"metadata.max.age.ms":      180000,
		})

	// KafkaTopicProbeConsumer receives a message when a new topic is created
	internal.SetupKafkaTopicProbeConsumer(
		kafka.ConfigMap{
			"bootstrap.servers":        KafkaBoostrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"group.id":                 "mqtt-kafka-bridge-topic-probe",
			"enable.auto.commit":       true,
			"auto.offset.reset":        "earliest",
			// "debug":                    "security,broker",
			"topic.metadata.refresh.interval.ms": "30000",
		})

	err = internal.CreateTopicIfNotExists(KafkaBaseTopic)
	if err != nil {
		zap.S().Fatalf("Failed to create topic %s: %v", KafkaBaseTopic, err)
	}

	zap.S().Debugf("Start Queue processors")
	go internal.StartEventHandler("MQTTKafkaBridge", internal.KafkaProducer.Events(), nil)
	processIncomingMessages()
	processOutgoingMessages()
	go kafkaToQueue(KafkaTopic)

	// Start topic probe processor
	zap.S().Debugf("Starting TP queue processor")
	topicProbeProcessorChannel := make(chan *kafka.Message, 100)

	go internal.ProcessKafkaTopicProbeQueue("[TP]", topicProbeProcessorChannel, nil)
	go internal.StartEventHandler("[TP]", internal.KafkaTopicProbeConsumer.Events(), nil)

	go internal.StartTopicProbeQueueProcessor(topicProbeProcessorChannel)
	zap.S().Debugf("Started TP queue processor")

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
		zap.S().Infof("Received SIGTERM: %v", sig)
		zap.S().Infof("Stacktrace: %v", string(debug.Stack()))

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	go ReportStats(RawLruSize, LruSize)

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

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

func ReportStats(rawLruSize int, LruSize int) {
	lastConfirmed := 0.0
	for !ShuttingDown {
		zap.S().Infof(
			"Reporting stats"+
				"| MQTT->Kafka queue length: %d"+
				"| Kafka->MQTT queue length: %d"+
				"| Produced Kafka messages: %f"+
				"| Produced Kafka messages/s: %f"+
				"| Produced MQTT messages: %d"+
				"| Produced MQTT messages/s: %f"+
				"| Message LRU size: %d/%d"+
				"| Raw Message LRU size: %d/%d",
			mqttIncomingQueue.Length(),
			mqttOutGoingQueue.Length(),
			internal.KafkaConfirmed,
			(internal.KafkaConfirmed-lastConfirmed)/5,
			SentMQTTMessages,
			float64(SentMQTTMessages)/5,
			MessageLRU.Len(),
			LruSize,
			RawMessageLRU.Len(),
			rawLruSize,
		)
		lastConfirmed = internal.KafkaConfirmed
		time.Sleep(internal.FiveSeconds)
	}
}
