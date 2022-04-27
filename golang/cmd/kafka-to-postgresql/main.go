package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	r "k8s.io/apimachinery/pkg/api/resource"
	"math"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
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
		HITopic = strings.ReplaceAll(HITopic, `\\`, `\`)
		zap.S().Infof("High integrity topic is set to %s", HITopic)
	}
	HTTopic := os.Getenv("KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC")
	if HTTopic == "" {
		zap.S().Warnf("KAFKA_HIGH_THROUGHPUT_LISTEN_TOPIC not set")
	} else {
		HighThroughputEnabled = true
		HTTopic = strings.ReplaceAll(HTTopic, `\\`, `\`)
		zap.S().Infof("High throughput topic is set to %s", HTTopic)
	}

	// If neither high-integrity nor high-throughput topic is configured, panic
	if !HighThroughputEnabled && !HighIntegrityEnabled {
		panic("No topics enabled")
	}

	// Combining enable.auto.commit and enable.auto.offset.store
	// leads to better performance.
	// Processed message now will be stored locally and then automatically committed to Kafka.
	// This still provides the at-least-once guarantee.
	if HighIntegrityEnabled {
		SetupHIKafka(kafka.ConfigMap{
			"bootstrap.servers":        KafkaBoostrapServer,
			"security.protocol":        "plaintext",
			"group.id":                 "kafka-to-postgresql-hi-processor",
			"enable.auto.commit":       true,
			"enable.auto.offset.store": false,
			"auto.offset.reset":        "earliest",
		})
	}

	// HT uses enable.auto.commit=true for increased performance.
	if HighThroughputEnabled {
		SetupHTKafka(kafka.ConfigMap{
			"bootstrap.servers":  KafkaBoostrapServer,
			"security.protocol":  "plaintext",
			"group.id":           "kafka-to-postgresql-ht-processor",
			"enable.auto.commit": true,
			"auto.offset.reset":  "earliest",
		})
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
	InitCache(allowedMemorySize / 4)
	internal.InitMessageCache(allowedMemorySize / 4)

	zap.S().Debugf("Starting queue processor")

	// Start HI related processors
	if HighIntegrityEnabled {
		zap.S().Debugf("Starting HI queue processor")
		highIntegrityProcessorChannel = make(chan *kafka.Message, 100)
		highIntegrityPutBackChannel = make(chan internal.PutBackChanMsg, 200)
		highIntegrityCommitChannel = make(chan *kafka.Message)
		highIntegrityEventChannel := HIKafkaProducer.Events()
		go internal.KafkaStartPutbackProcessor("[HI]", highIntegrityPutBackChannel, HIKafkaProducer)
		go internal.KafkaProcessQueue("[HI]", HITopic, highIntegrityProcessorChannel, HIKafkaConsumer, highIntegrityPutBackChannel)
		go internal.KafkaStartCommitProcessor("[HI]", highIntegrityCommitChannel, HIKafkaConsumer)
		go startHighIntegrityQueueProcessor()
		go internal.StartEventHandler("[HI]", highIntegrityEventChannel, highIntegrityPutBackChannel)
		zap.S().Debugf("Started HI queue processor")
	}

	// Start HT related processors
	if HighThroughputEnabled {
		zap.S().Debugf("Starting HT queue processor")
		highThroughputProcessorChannel = make(chan *kafka.Message, 1000)
		highThroughputPutBackChannel = make(chan internal.PutBackChanMsg, 200)
		highThroughputEventChannel := HIKafkaProducer.Events()
		// HT has no commit channel, it uses auto commit
		go internal.KafkaStartPutbackProcessor("[HT]", highThroughputPutBackChannel, HTKafkaProducer)
		go internal.KafkaProcessQueue("[HT]", HTTopic, highThroughputProcessorChannel, HTKafkaConsumer, highThroughputPutBackChannel)
		go startHighThroughputQueueProcessor()
		go internal.StartEventHandler("[HI]", highThroughputEventChannel, highIntegrityPutBackChannel)

		go startProcessValueQueueAggregator()
		go startProcessValueStringQueueAggregator()
		zap.S().Debugf("Started HT queue processor")
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

	// The following code keeps the memory usage low
	debug.SetGCPercent(10)
	go func() {
		allowedSeventyFivePerc := uint64(float64(allowedMemorySize) * 0.9)
		allowedNintyPerc := uint64(float64(allowedMemorySize) * 0.75)
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.Alloc > allowedNintyPerc {
				zap.S().Errorf("Memory usage is too high: %d bytes, slowing ingress !", m.TotalAlloc)
				internal.NearMemoryLimit = true
				debug.FreeOSMemory()
				time.Sleep(internal.FiveSeconds)
			}
			if m.Alloc > allowedSeventyFivePerc {
				zap.S().Errorf("Memory usage is high: %d bytes !", m.TotalAlloc)
				internal.NearMemoryLimit = false
				runtime.GC()
				time.Sleep(internal.FiveSeconds)
			} else {
				internal.NearMemoryLimit = false
				time.Sleep(internal.OneSecond)
			}
		}
	}()

	go PerformanceReport()
	select {
	case <-internal.ShutdownMainChan:
		zap.S().Info("Shutdown signal received from kafka")
		ShutdownApplicationGraceful()
		return
	} // block forever
}

var ShuttingDown bool

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	if ShuttingDown {
		// Already shutting down
		return
	}

	zap.S().Infof("Shutting down application")
	ShuttingDown = true
	internal.KafkaShuttingDown = true
	// Important, allows high load processors to finish
	time.Sleep(time.Second * 5)

	if HighIntegrityEnabled {
		zap.S().Debugf("Cleaning up high integrity processor channel (%d)", len(highIntegrityProcessorChannel))
		if !internal.DrainChannel("[HT]", highIntegrityProcessorChannel, highIntegrityPutBackChannel) {
			time.Sleep(internal.FiveSeconds)
		}

		time.Sleep(internal.OneSecond)

		for len(highIntegrityPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(highIntegrityPutBackChannel))
			time.Sleep(internal.OneSecond)
		}
	}

	// This is behind HI to allow a higher chance of a clean shutdown
	if HighThroughputEnabled {
		zap.S().Debugf("Cleaning up high throughput processor channel (%d)", len(highThroughputProcessorChannel))
		if !internal.DrainChannel("[HIGH_THROUGHPUT]", highThroughputProcessorChannel, highThroughputPutBackChannel) {
			time.Sleep(internal.FiveSeconds)
		}
		if !internal.DrainChannel("[HIGH_THROUGHPUT]", processValueChannel, highThroughputPutBackChannel) {
			time.Sleep(internal.FiveSeconds)
		}
		if !internal.DrainChannel("[HIGH_THROUGHPUT]", processValueStringChannel, highThroughputPutBackChannel) {
			time.Sleep(internal.FiveSeconds)
		}

		time.Sleep(internal.OneSecond)

		for len(highThroughputPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(highThroughputPutBackChannel))
			time.Sleep(internal.OneSecond)
		}
	}
	internal.KafkaShutdownPutback = true

	time.Sleep(internal.OneSecond)

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

func PerformanceReport() {
	lastCommits := float64(0)
	lastMessages := float64(0)
	lastPutbacks := float64(0)
	lastConfirmed := float64(0)
	sleepS := 10.0
	for !ShuttingDown {
		preExecutionTime := time.Now()
		commitsPerSecond := (internal.KafkaCommits - lastCommits) / sleepS
		messagesPerSecond := (internal.KafkaMessages - lastMessages) / sleepS
		putbacksPerSecond := (internal.KafkaPutBacks - lastPutbacks) / sleepS
		confirmedPerSecond := (internal.KafkaConfirmed - lastConfirmed) / sleepS
		lastCommits = internal.KafkaCommits
		lastMessages = internal.KafkaMessages
		lastPutbacks = internal.KafkaPutBacks
		lastConfirmed = internal.KafkaConfirmed

		zap.S().Infof("Performance report"+
			"\nKafkaCommits: %f, KafkaCommits/s: %f"+
			"\nKafkaMessages: %f, KafkaMessages/s: %f"+
			"\nKafkaPutBacks: %f, KafkaPutBacks/s: %f"+
			"\nKafkaConfirmed: %f, KafkaConfirmed/s: %f"+
			"\n[HI] Processor queue length: %d"+
			"\n[HI] PutBack queue length: %d"+
			"\n[HI] Commit queue length: %d"+
			"\nMessagecache hitrate %f"+
			"\nDbcache hitrate %f"+
			"\n[HT] ProcessValue queue lenght: %d"+
			"\n[HT] ProcessValueString queue lenght: %d"+
			"\n[HT] Processor queue length: %d"+
			"\n[HT] PutBack queue length: %d",
			internal.KafkaCommits, commitsPerSecond,
			internal.KafkaMessages, messagesPerSecond,
			internal.KafkaPutBacks, putbacksPerSecond,
			internal.KafkaConfirmed, confirmedPerSecond,
			len(highIntegrityProcessorChannel),
			len(highIntegrityPutBackChannel),
			len(highIntegrityCommitChannel),
			internal.Messagecache.HitRate(),
			dbcache.HitRate(),
			len(processValueChannel),
			len(processValueStringChannel),
			len(highThroughputProcessorChannel),
			len(highThroughputPutBackChannel),
		)

		if internal.KafkaCommits > math.MaxFloat64/2 || lastCommits > math.MaxFloat64/2 {
			internal.KafkaCommits = 0
			lastCommits = 0
			zap.S().Warnf("Resetting commit statistics")
		}

		if internal.KafkaMessages > math.MaxFloat64/2 || lastMessages > math.MaxFloat64/2 {
			internal.KafkaMessages = 0
			lastMessages = 0
			zap.S().Warnf("Resetting message statistics")
		}

		if internal.KafkaPutBacks > math.MaxFloat64/2 || lastPutbacks > math.MaxFloat64/2 {
			internal.KafkaPutBacks = 0
			lastPutbacks = 0
			zap.S().Warnf("Resetting putback statistics")
		}

		if internal.KafkaConfirmed > math.MaxFloat64/2 || lastConfirmed > math.MaxFloat64/2 {
			internal.KafkaConfirmed = 0
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
