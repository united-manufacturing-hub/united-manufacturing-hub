package main

// This package displays all Kafka messages, useful for debugging the stack

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
	"time"
)

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

	zap.S().Infof("This is kafka-debug build date: %s", buildtime)

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
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
		"group.id":                 "kafka-debug",
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

	internal.CloseKafka()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
