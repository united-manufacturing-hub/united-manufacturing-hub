package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	r "k8s.io/apimachinery/pkg/api/resource"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"
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

	allowedMemorySize := 1073741824 // 1GB
	if os.Getenv("MEMORY_REQUEST") != "" {
		memoryRequest := r.MustParse(os.Getenv("MEMORY_REQUEST"))
		i, b := memoryRequest.AsInt64()
		if b {
			allowedMemorySize = int(i) //truncated !
		}
	}
	zap.S().Infof("Allowed memory size is %d", allowedMemorySize)

	// InitCache is initialized with 1Gb of memory for each cache
	internal.InitMessageCache(allowedMemorySize / 4)

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

		go internal.StartPutbackProcessor("[AC]", ActivityPutBackChannel, ActivityKafkaProducer, ActivityCommitChannel)
		go internal.ProcessKafkaQueue("[AC]", activityTopic, ActivityProcessorChannel, ActivityKafkaConsumer, ActivityPutBackChannel, ShutdownApplicationGraceful)
		go internal.StartCommitProcessor("[AC]", ActivityCommitChannel, ActivityKafkaConsumer)
		go internal.StartEventHandler("[AC]", activityEventChannel, ActivityPutBackChannel)
		go startActivityProcessor()
	}
	AnomalyEnabled = os.Getenv("ANOMALY_ENABLED") == "true"

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

		go internal.StartPutbackProcessor("[AN]", AnomalyPutBackChannel, AnomalyKafkaProducer, ActivityCommitChannel)
		go internal.ProcessKafkaQueue("[AN]", anomalyTopic, AnomalyProcessorChannel, AnomalyKafkaConsumer, AnomalyPutBackChannel, ShutdownApplicationGraceful)
		go internal.StartCommitProcessor("[AN]", AnomalyCommitChannel, AnomalyKafkaConsumer)
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

	select {} // block forever
}

var ShuttingDown bool

func ShutdownApplicationGraceful() {
	if ShuttingDown {
		return
	}
	zap.S().Info("Shutting down application")
	ShuttingDown = true

	internal.ShuttingDownKafka = true
	// Important, allows high load processors to finish
	time.Sleep(time.Second * 5)

	if ActivityEnabled {
		if !internal.DrainChannelSimple(ActivityProcessorChannel, ActivityPutBackChannel) {
			time.Sleep(internal.FiveSeconds)
		}
		time.Sleep(internal.OneSecond)

		for len(ActivityPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(ActivityPutBackChannel))
			time.Sleep(internal.OneSecond)
		}
	}

	if AnomalyEnabled {
		if !internal.DrainChannelSimple(AnomalyProcessorChannel, AnomalyPutBackChannel) {
			time.Sleep(internal.FiveSeconds)
		}
		time.Sleep(internal.OneSecond)

		for len(AnomalyPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(AnomalyPutBackChannel))
			time.Sleep(internal.OneSecond)
		}
	}

	internal.ShutdownPutback = true
	time.Sleep(internal.OneSecond)

	if ActivityEnabled {
		CloseActivityKafka()
	}
	if AnomalyEnabled {
		CloseAnomalyKafka()
	}

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
