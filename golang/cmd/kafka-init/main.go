package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net"
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
	zap.S().Infof("This is kafka-init build date: %s", buildtime)

	internal.Initfgtrace()

	// Read environment variables for Kafka
	KafkaBoostrapServer, KafkaBoostrapServerEnvSet := os.LookupEnv("KAFKA_BOOTSTRAP_SERVER")
	if !KafkaBoostrapServerEnvSet {
		zap.S().Fatal("Kafka Bootstrap Server (KAFKA_BOOTSTRAP_SERVER) must be set")
	}
	zap.S().Infof("KafkaBoostrapServer: %s", KafkaBoostrapServer)
	// Semicolon separated list of topic to create
	KafkaTopics, KafkaTopicsEnvSet := os.LookupEnv("KAFKA_TOPICS")
	if !KafkaTopicsEnvSet {
		zap.S().Fatal("Kafka Topics (KAFKA_TOPICS) must be set")
	}
	zap.S().Infof("KafkaTopics: %s", KafkaTopics)

	zap.S().Debugf("Setting up Kafka")
	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
		zap.S().Infof("Using SSL")
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
			zap.S().Fatalf("Error opening ca.crt: %s", err)
		}
	} else {
		zap.S().Infof("Using plaintext")
	}

	timeout := 10 * time.Second
	conn, err := net.DialTimeout("tcp", KafkaBoostrapServer, timeout)
	if err != nil {
		zap.S().Errorf("site unreachable, error: %v", err)
	} else {
		zap.S().Infof("Site reachable, connection: %v", conn)
	}
	defer func(conn net.Conn) {
		err = conn.Close()
		if err != nil {
			zap.S().Errorf("Error closing connection: %s", err)
		}
	}(conn)

	internal.SetupKafka(
		kafka.ConfigMap{
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"bootstrap.servers":        KafkaBoostrapServer,
			"group.id":                 "kafka-init",
			"metadata.max.age.ms":      180000,
		})

	initKafkaTopics(KafkaTopics)

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

	ShutdownApplicationGraceful()
}

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")

	internal.CloseKafka()

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
