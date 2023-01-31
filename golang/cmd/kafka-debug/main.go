package main

// This package displays all Kafka messages, useful for debugging the stack

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var buildtime string

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)
	zap.S().Infof("This is kafka-debug build date: %s", buildtime)

	internal.Initfgtrace()

	// Read environment variables for Kafka
	KafkaBoostrapServer, KafkaBoostrapServerEnvSet := os.LookupEnv("KAFKA_BOOTSTRAP_SERVER")
	if !KafkaBoostrapServerEnvSet {
		zap.S().Fatal("Kafka Boostrap Server (KAFKA_BOOTSTRAP_SERVER) must be set")
	}
	zap.S().Debugf("Setting up Kafka")

	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
		securityProtocol = "ssl"

		_, err := os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatalf("Error opening key file: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Error opening certificate: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Error opening CA certificate: %s", err)
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
			"group.id":                 "kafka-debug",
			"metadata.max.age.ms":      180000,
		})

	zap.S().Debugf("Start Queue processors")
	go startDebugger()

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
		zap.S().Infof("Received SIGTERM", sig)

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

	internal.CloseKafka()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
