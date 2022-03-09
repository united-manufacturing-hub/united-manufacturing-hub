package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/kafka_helper"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
)

var buildtime string

func main() {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is kafka-init build date: %s", buildtime)

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	// Semicolon seperated list of topic to create
	KafkaTopics := os.Getenv("KAFKA_TOPICS")

	zap.S().Debugf("Setting up Kafka")
	kafka_helper.SetupKafka(kafka.ConfigMap{
		"bootstrap.servers": KafkaBoostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "kafka-init",
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
		zap.S().Infof("Recieved SIGTERM", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {} // block forever
}

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")

	kafka_helper.CloseKafka()

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
