package main

// This package displays all Kafka messages, useful for debugging the stack

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var kafkaConsumerClient *kafka.Consumer

var buildtime string

func main() {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is kafka-debug build date: %s", buildtime)

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	zap.S().Debugf("Setting up Kafka")
	kafkaConsumerClient = setupKafka(KafkaBoostrapServer)

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
	if kafkaConsumerClient != nil {
		kafkaConsumerClient.Close()
	}

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
