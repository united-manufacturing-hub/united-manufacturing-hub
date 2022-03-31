package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"
)

var buildtime string

var f *os.File

var HighIntegrityEnabled = false
var HighThroughputEnabled = false
var HITopic string
var HTTopic string

func main() {
	// BEGIN PROFILER
	var perr error
	f, perr = os.Create("cpu.pprof")
	if perr != nil {
		panic(perr)
	}
	err := pprof.StartCPUProfile(f)
	if err != nil {
		panic(err)
	}
	// END PROFILER

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

	zap.S().Debugf("Setting up database with %s %d %s %s %s %s", PQHost, PQPort, PQUser, PQPassword, PWDBName, PQSSLMode)

	SetupDB(PQUser, PQPassword, PWDBName, PQHost, PQPort, health, dryRun, PQSSLMode)

	zap.S().Debugf("Setting up Kafka")
	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	if KafkaBoostrapServer == "" {
		panic("KAFKA_BOOSTRAP_SERVER not set")
	}
	HITopic = os.Getenv("KAFKA_HIGH_INTEGRITY_LISTEN_TOPIC")
	if HITopic == "" {
		zap.S().Warnf("KAFKA_HIGH_INTEGRITY_LISTEN_TOPIC not set")
	} else {
		HighIntegrityEnabled = true
	}
	HTTopic = os.Getenv("KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC")
	if HTTopic == "" {
		zap.S().Warnf("KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC not set")
	} else {
		HighThroughputEnabled = true
	}

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

	SetupHTKafka(kafka.ConfigMap{
		"bootstrap.servers":      KafkaBoostrapServer,
		"security.protocol":      "plaintext",
		"group.id":               "kafka-to-postgresql-ht-processor",
		"enable.auto.commit":     true,
		"auto.offset.reset":      "earliest",
		"statistics.interval.ms": 1000 * 10,
	})

	// 1GB, 1GB
	InitCache(1073741824, 1073741824)

	zap.S().Debugf("Starting queue processor")

	if HighIntegrityEnabled {
		highIntegrityProcessorChannel = make(chan *kafka.Message, 100)
		highIntegrityPutBackChannel = make(chan PutBackChanMsg, 200)
		highIntegrityCommitChannel = make(chan *kafka.Message)
		go startPutbackProcessor("[HI]", highIntegrityPutBackChannel, HIKafkaProducer)
		go processKafkaQueue("[HI]", HITopic, highIntegrityProcessorChannel, HIKafkaConsumer, highIntegrityPutBackChannel)
		go startCommitProcessor("[HI]", highIntegrityCommitChannel, HIKafkaConsumer)
		go startHighIntegrityQueueProcessor()
	}

	if HighThroughputEnabled {
		highThroughputProcessorChannel = make(chan *kafka.Message, 1000)
		highThroughputPutBackChannel = make(chan PutBackChanMsg, 200)
		// HT has no commit channel, it uses auto commit
		go startPutbackProcessor("[HT]", highThroughputPutBackChannel, HTKafkaProducer)
		go processKafkaQueue("[HT]", HTTopic, highThroughputProcessorChannel, HTKafkaConsumer, highThroughputPutBackChannel)
		go startHighThroughputQueueProcessor()

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

	pprof.StopCPUProfile()
	f.Close()

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

var Commits = 0
var Messages = 0
var PutBacks = 0

func PerformanceReport() {
	lastCommits := 0
	lastMessages := 0
	lastPutbacks := 0
	sleepS := 10.0
	for !ShuttingDown {
		preExecutionTime := time.Now()
		commitsPerSecond := float64(Commits-lastCommits) / sleepS
		messagesPerSecond := float64(Messages-lastMessages) / sleepS
		putbacksPerSecond := float64(PutBacks-lastPutbacks) / sleepS
		lastCommits = Commits
		lastMessages = Messages
		lastPutbacks = PutBacks

		zap.S().Infof("Performance report"+
			"\nCommits: %d, Commits/s: %f"+
			"\nMessages: %d, Messages/s: %f"+
			"\nPutBacks: %d, PutBacks/s: %f"+
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

		postExecutionTime := time.Now()
		ExecutionTimeDiff := postExecutionTime.Sub(preExecutionTime).Seconds()
		if ExecutionTimeDiff <= 0 {
			continue
		}
		time.Sleep(time.Second * time.Duration(sleepS-ExecutionTimeDiff))
	}
}
