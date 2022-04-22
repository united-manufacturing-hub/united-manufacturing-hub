package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
)

var buildtime string

var ActivityEnabled bool

var AnomalyEnabled bool

func main() {

	var logger *zap.Logger
	if os.Getenv("LOGGING_LEVEL") == "DEVELOPMENT" {
		logger, _ = zap.NewDevelopment()
	} else {

		logger, _ = zap.NewProduction()
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is kafka-state-detector build date: %s", buildtime)

	zap.S().Debugf("Setting up Kafka")
	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	if KafkaBoostrapServer == "" {
		panic("KAFKA_BOOSTRAP_SERVER not set")
	}

	ActivityEnabled = os.Getenv("ACTIVITY_ENABLED") == "true"

	if ActivityEnabled {
		SetupActivityKafka(kafka.ConfigMap{
			"bootstrap.servers":        KafkaBoostrapServer,
			"security.protocol":        "plaintext",
			"group.id":                 "kafka-state-detector-activity",
			"enable.auto.commit":       true,
			"enable.auto.offset.store": false,
			"auto.offset.reset":        "earliest",
		})

		ActivityProcessorChannel = make(chan *kafka.Message, 100)
		ActivityCommitChannel = make(chan *kafka.Message)
		activityEventChannel := ActivityKafkaProducer.Events()
		activityTopic := "^ia\\.\\w*\\.\\w*\\.\\w*\\.activity$"

		go internal.KafkaStartPutbackProcessor("[AC]", ActivityPutBackChannel, ActivityKafkaProducer)
		go internal.KafkaProcessQueue("[AC]", activityTopic, ActivityProcessorChannel, ActivityKafkaConsumer, ActivityPutBackChannel)
		go internal.KafkaStartCommitProcessor("[AC]", ActivityCommitChannel, ActivityKafkaConsumer)
		go internal.StartEventHandler("[AC]", activityEventChannel, ActivityPutBackChannel)
		go startActivityProcessor()
	}

	if AnomalyEnabled {
		SetupAnomalyKafka(kafka.ConfigMap{
			"security.protocol":  "plaintext",
			"group.id":           fmt.Sprintf("kafka-state-detector-anomaly-%d", rand.Uint64()),
			"enable.auto.commit": true,
			"auto.offset.reset":  "earliest",
		})

		AnomalyProcessorChannel = make(chan *kafka.Message, 100)
		AnomalyCommitChannel = make(chan *kafka.Message)
		anomalyEventChannel := AnomalyKafkaProducer.Events()
		anomalyTopic := "^ia\\.\\w*\\.\\w*\\.\\w*\\.activity$"

		go internal.KafkaStartPutbackProcessor("[AN]", AnomalyPutBackChannel, AnomalyKafkaProducer)
		go internal.KafkaProcessQueue("[AN]", anomalyTopic, AnomalyProcessorChannel, AnomalyKafkaConsumer, AnomalyPutBackChannel)
		go internal.KafkaStartCommitProcessor("[AN]", AnomalyCommitChannel, AnomalyKafkaConsumer)
		go internal.StartEventHandler("[AN]", anomalyEventChannel, AnomalyPutBackChannel)
		go startAnomalyActivityProcessor()
	}

	if !ActivityEnabled && !AnomalyEnabled {
		panic("No activity or anomaly detection enabled")
	}

	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	// It's important to handle both signals, allowing Kafka to shut down gracefully !
	// If this is not possible, it will attempt to rebalance itself, which will increase startup time
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Recieved SIG %v", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {
	case <-internal.ShutdownMainChan:
		zap.S().Info("Shutdown signal received from kafka")
		ShutdownApplicationGraceful()
		return
	} // block forever
}

var ShuttingDown bool

func ShutdownApplicationGraceful() {
	if ShuttingDown {
		return
	}
	zap.S().Info("Shutting down application")
	ShuttingDown = true
}
