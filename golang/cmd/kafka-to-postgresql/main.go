package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var buildtime string

// HighIntegrityEnabled is true, when a high integrity topic has been configured (KAFKA_HIGH_INTEGRITY_LISTEN_TOPIC)
var HighIntegrityEnabled = false

// HighThroughputEnabled is true, when a high throughput topic has been configured (KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC)
var HighThroughputEnabled = false

func main() {
	// Setup logger and set as global
	var logger *zap.Logger
	if os.Getenv("LOGGING_LEVEL") == "DEVELOPMENT" {
		logger, _ = zap.NewDevelopment()
	} else {

		logger, _ = zap.NewProduction()
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is kafka-to-postgresql build date: %s", buildtime)

	dryRun := os.Getenv("DRY_RUN")

	// Prometheus
	metricsPath := "/metrics"
	metricsPort := ":2112"
	zap.S().Debugf("Setting up metrics %s %v", metricsPath, metricsPort)

	http.Handle(metricsPath, promhttp.Handler())
	go http.ListenAndServe(metricsPort, nil)

	// Prometheus
	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go http.ListenAndServe("0.0.0.0:8086", health)

	// Postgres
	PQHost := os.Getenv("POSTGRES_HOST")
	PQPort := 5432
	PQUser := os.Getenv("POSTGRES_USER")
	PQPassword := os.Getenv("POSTGRES_PASSWORD")
	PWDBName := os.Getenv("POSTGRES_DATABASE")
	PQSSLMode := os.Getenv("POSTGRES_SSLMODE")
	if PQSSLMode == "" {
		PQSSLMode = "require"
	} else {
		zap.S().Warnf("Postgres SSL mode is set to %s", PQSSLMode)
	}

	SetupDB(PQUser, PQPassword, PWDBName, PQHost, PQPort, health, dryRun, PQSSLMode)

	zap.S().Debugf("Setting up Kafka")
	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	if KafkaBoostrapServer == "" {
		panic("KAFKA_BOOSTRAP_SERVER not set")
	}
	HITopic := os.Getenv("KAFKA_HIGH_INTEGRITY_LISTEN_TOPIC")
	if HITopic == "" {
		zap.S().Warnf("KAFKA_HIGH_INTEGRITY_LISTEN_TOPIC not set")
	} else {
		HighIntegrityEnabled = true
	}
	HTTopic := os.Getenv("KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC")
	if HTTopic == "" {
		zap.S().Warnf("KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC not set")
	} else {
		HighThroughputEnabled = true
	}

	// If neither high-integrity nor high-throughput topic is configured, panic
	if !HighThroughputEnabled && !HighIntegrityEnabled {
		panic("No topics enabled")
	}

	// Combining enable.auto.commit and enable.auto.offset.store
	// leads to better performance.
	// Processed message now will be stored locally and then automatically committed to Kafka.
	// This still provides the at-least-once guarantee.
	SetupHIKafka(kafka.ConfigMap{
		"bootstrap.servers":        KafkaBoostrapServer,
		"security.protocol":        "plaintext",
		"group.id":                 "kafka-to-postgresql-hi-processor",
		"enable.auto.commit":       true,
		"enable.auto.offset.store": false,
		"auto.offset.reset":        "earliest",
		"statistics.interval.ms":   1000 * 10,
	})

	// HT uses enable.auto.commit=true for increased performance.
	SetupHTKafka(kafka.ConfigMap{
		"bootstrap.servers":      KafkaBoostrapServer,
		"security.protocol":      "plaintext",
		"group.id":               "kafka-to-postgresql-ht-processor",
		"enable.auto.commit":     true,
		"auto.offset.reset":      "earliest",
		"statistics.interval.ms": 1000 * 10,
	})

	// InitCache is initialized with 1Gb of memory for each cache
	InitCache(1073741824, 1073741824)

	zap.S().Debugf("Starting queue processor")

	// Start HI related processors
	if HighIntegrityEnabled {
		highIntegrityProcessorChannel = make(chan *kafka.Message, 100)
		highIntegrityPutBackChannel = make(chan PutBackChanMsg, 200)
		highIntegrityCommitChannel = make(chan *kafka.Message)
		highIntegrityEventChannel := HIKafkaProducer.Events()
		go startPutbackProcessor("[HI]", highIntegrityPutBackChannel, HIKafkaProducer)
		go processKafkaQueue("[HI]", HITopic, highIntegrityProcessorChannel, HIKafkaConsumer, highIntegrityPutBackChannel)
		go startCommitProcessor("[HI]", highIntegrityCommitChannel, HIKafkaConsumer)
		go startHighIntegrityQueueProcessor()
		go startEventHandler("[HI]", highIntegrityEventChannel, highIntegrityPutBackChannel)
	}

	// Start HT related processors
	if HighThroughputEnabled {
		highThroughputProcessorChannel = make(chan *kafka.Message, 1000)
		highThroughputPutBackChannel = make(chan PutBackChanMsg, 200)
		highThroughputEventChannel := HIKafkaProducer.Events()
		// HT has no commit channel, it uses auto commit
		go startPutbackProcessor("[HT]", highThroughputPutBackChannel, HTKafkaProducer)
		go processKafkaQueue("[HT]", HTTopic, highThroughputProcessorChannel, HTKafkaConsumer, highThroughputPutBackChannel)
		go startHighThroughputQueueProcessor()
		go startEventHandler("[HI]", highThroughputEventChannel, highIntegrityPutBackChannel)

		go startProcessValueQueueAggregator()
		go startProcessValueStringQueueAggregator()
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

	go PerformanceReport()
	select {} // block forever
}

var ShuttingDown bool
var ShutdownPutback bool

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	ShuttingDown = true
	// Important, allows high load processors to finish
	time.Sleep(time.Second * 5)

	if HighIntegrityEnabled {
		zap.S().Debugf("Cleaning up high integrity processor channel (%d)", len(highIntegrityProcessorChannel))
		if !DrainChannel("[HT]", highIntegrityProcessorChannel, highIntegrityPutBackChannel) {
			time.Sleep(5 * time.Second)
		}

		time.Sleep(1 * time.Second)

		for len(highIntegrityPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(highIntegrityPutBackChannel))
			time.Sleep(1 * time.Second)
		}
	}

	// This is behind HI to allow a higher chance of a clean shutdown
	if HighThroughputEnabled {
		zap.S().Debugf("Cleaning up high throughput processor channel (%d)", len(highThroughputProcessorChannel))
		if !DrainChannel("[HIGH_THROUGHPUT]", highThroughputProcessorChannel, highThroughputPutBackChannel) {
			time.Sleep(5 * time.Second)
		}
		if !DrainChannel("[HIGH_THROUGHPUT]", processValueChannel, highThroughputPutBackChannel) {
			time.Sleep(5 * time.Second)
		}
		if !DrainChannel("[HIGH_THROUGHPUT]", processValueStringChannel, highThroughputPutBackChannel) {
			time.Sleep(5 * time.Second)
		}

		time.Sleep(1 * time.Second)

		for len(highThroughputPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(highThroughputPutBackChannel))
			time.Sleep(1 * time.Second)
		}
	}
	ShutdownPutback = true

	time.Sleep(1 * time.Second)

	if HighIntegrityEnabled {
		CloseHIKafka()
	}
	if HighThroughputEnabled {
		CloseHTKafka()
	}

	ShutdownDB()

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

// Commits is a counter for the number of commits done (to the db), this is used for stats only
var Commits = float64(0)

// Messages is a counter for the number of messages processed, this is used for stats only
var Messages = float64(0)

// PutBacks is a counter for the number of messages returned to kafka, this is used for stats only
var PutBacks = float64(0)

// Confirmed is a counter for the number of messages confirmed to kafka, this is used for stats only
var Confirmed = float64(0)

func PerformanceReport() {
	lastCommits := float64(0)
	lastMessages := float64(0)
	lastPutbacks := float64(0)
	lastConfirmed := float64(0)
	sleepS := 10.0
	for !ShuttingDown {
		preExecutionTime := time.Now()
		commitsPerSecond := (Commits - lastCommits) / sleepS
		messagesPerSecond := (Messages - lastMessages) / sleepS
		putbacksPerSecond := (PutBacks - lastPutbacks) / sleepS
		confirmedPerSecond := (Confirmed - lastConfirmed) / sleepS
		lastCommits = Commits
		lastMessages = Messages
		lastPutbacks = PutBacks
		lastConfirmed = Confirmed

		zap.S().Infof("Performance report"+
			"\nCommits: %f, Commits/s: %f"+
			"\nMessages: %f, Messages/s: %f"+
			"\nPutBacks: %f, PutBacks/s: %f"+
			"\nConfirmed: %f, Confirmed/s: %f"+
			"\n[HI] Processor queue length: %d"+
			"\n[HI] PutBack queue length: %d"+
			"\n[HI] Commit queue length: %d"+
			"\nMessagecache hitrate %f"+
			"\nDbcache hitrate %f"+
			"\n[HT] ProcessValue queue lenght: %d"+
			"\n[HT] ProcessValueString queue lenght: %d"+
			"\n[HT] Processor queue length: %d"+
			"\n[HT] PutBack queue length: %d",
			Commits, commitsPerSecond,
			Messages, messagesPerSecond,
			PutBacks, putbacksPerSecond,
			Confirmed, confirmedPerSecond,
			len(highIntegrityProcessorChannel),
			len(highIntegrityPutBackChannel),
			len(highIntegrityCommitChannel),
			messagecache.HitRate(),
			dbcache.HitRate(),
			len(processValueChannel),
			len(processValueStringChannel),
			len(highThroughputProcessorChannel),
			len(highThroughputPutBackChannel),
		)

		if Commits > math.MaxFloat64/2 || lastCommits > math.MaxFloat64/2 {
			Commits = 0
			lastCommits = 0
			zap.S().Warnf("Resetting commit statistics")
		}

		if Messages > math.MaxFloat64/2 || lastMessages > math.MaxFloat64/2 {
			Messages = 0
			lastMessages = 0
			zap.S().Warnf("Resetting message statistics")
		}

		if PutBacks > math.MaxFloat64/2 || lastPutbacks > math.MaxFloat64/2 {
			PutBacks = 0
			lastPutbacks = 0
			zap.S().Warnf("Resetting putback statistics")
		}

		if Confirmed > math.MaxFloat64/2 || lastConfirmed > math.MaxFloat64/2 {
			Confirmed = 0
			lastConfirmed = 0
			zap.S().Warnf("Resetting confirmed statistics")
		}
		postExecutionTime := time.Now()
		ExecutionTimeDiff := postExecutionTime.Sub(preExecutionTime).Seconds()
		if ExecutionTimeDiff <= 0 {
			continue
		}
		time.Sleep(time.Second * time.Duration(sleepS-ExecutionTimeDiff))
	}
}
