package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	kafka2 "github.com/united-manufacturing-hub/umh-lib/v2/kafka"
	"github.com/united-manufacturing-hub/umh-lib/v2/other"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	r "k8s.io/apimachinery/pkg/api/resource"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
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
	zap.S().Infof("This is kafka-to-postgresql build date: %s", buildtime)

	// pprof
	go http.ListenAndServe("localhost:1337", nil)

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

	securityProtocol := "plaintext"
	if other.EnvIsTrue("KAFKA_USE_SSL") {
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

	// Combining enable.auto.commit and enable.auto.offset.store
	// leads to better performance.
	// Processed message now will be stored locally and then automatically committed to Kafka.
	// This still provides the at-least-once guarantee.
	if HighIntegrityEnabled {
		SetupHIKafka(kafka.ConfigMap{
			"bootstrap.servers":        KafkaBoostrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/tls.key",
			"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
			"ssl.certificate.location": "/SSL_certs/tls.crt",
			"ssl.ca.location":          "/SSL_certs/ca.crt",
			"group.id":                 "kafka-to-postgresql-hi-processor",
			"enable.auto.commit":       true,
			"enable.auto.offset.store": false,
			"auto.offset.reset":        "earliest",
			//"debug":                    "security,broker",
		})
	}

	// HT uses enable.auto.commit=true for increased performance.
	if HighThroughputEnabled {
		SetupHTKafka(kafka.ConfigMap{
			"bootstrap.servers":        KafkaBoostrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/tls.key",
			"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
			"ssl.certificate.location": "/SSL_certs/tls.crt",
			"ssl.ca.location":          "/SSL_certs/ca.crt",
			"group.id":                 "kafka-to-postgresql-ht-processor",
			"enable.auto.commit":       true,
			"auto.offset.reset":        "earliest",
			//"debug":                    "security,broker",
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
	kafka2.InitMessageCache(allowedMemorySize / 4)

	zap.S().Debugf("Starting queue processor")

	// Start HI related processors
	if HighIntegrityEnabled {
		zap.S().Debugf("Starting HI queue processor")
		highIntegrityProcessorChannel = make(chan *kafka.Message, 100)
		highIntegrityPutBackChannel = make(chan kafka2.PutBackChanMsg, 200)
		highIntegrityCommitChannel = make(chan *kafka.Message)
		highIntegrityEventChannel := HIKafkaProducer.Events()

		go kafka2.StartPutbackProcessor("[HI]", highIntegrityPutBackChannel, HIKafkaProducer, highIntegrityCommitChannel)
		go kafka2.ProcessKafkaQueue("[HI]", HITopic, highIntegrityProcessorChannel, HIKafkaConsumer, highIntegrityPutBackChannel, ShutdownApplicationGraceful)
		go kafka2.StartCommitProcessor("[HI]", highIntegrityCommitChannel, HIKafkaConsumer)

		go startHighIntegrityQueueProcessor()
		go kafka2.StartEventHandler("[HI]", highIntegrityEventChannel, highIntegrityPutBackChannel)
		zap.S().Debugf("Started HI queue processor")
	}

	// Start HT related processors
	if HighThroughputEnabled {
		zap.S().Debugf("Starting HT queue processor")
		highThroughputProcessorChannel = make(chan *kafka.Message, 1000)
		highThroughputPutBackChannel = make(chan kafka2.PutBackChanMsg, 200)
		highThroughputEventChannel := HIKafkaProducer.Events()
		// HT has no commit channel, it uses auto commit

		go kafka2.StartPutbackProcessor("[HT]", highThroughputPutBackChannel, HTKafkaProducer, nil)
		go kafka2.ProcessKafkaQueue("[HT]", HTTopic, highThroughputProcessorChannel, HTKafkaConsumer, highThroughputPutBackChannel, nil)

		go startHighThroughputQueueProcessor()
		go kafka2.StartEventHandler("[HI]", highThroughputEventChannel, highIntegrityPutBackChannel)

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

	go kafka2.MemoryLimiter(allowedMemorySize)

	go PerformanceReport()
	select {} // block forever
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

	kafka2.ShuttingDownKafka = true

	// Important, allows high load processors to finish
	time.Sleep(time.Second * 5)

	if HighIntegrityEnabled {
		zap.S().Debugf("Cleaning up high integrity processor channel (%d)", len(highIntegrityProcessorChannel))

		if !kafka2.DrainChannelSimple(highIntegrityProcessorChannel, highIntegrityPutBackChannel) {

			time.Sleep(other.FiveSeconds)
		}

		time.Sleep(other.OneSecond)

		maxAttempts := 50
		attempt := 0

		for len(highIntegrityPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(highIntegrityPutBackChannel))
			time.Sleep(other.OneSecond)
			attempt++
			if attempt > maxAttempts {
				zap.S().Errorf("Putback channel is not empty after %d attempts, exiting", maxAttempts)
				break
			}
		}
	}

	// This is behind HI to allow a higher chance of a clean shutdown
	if HighThroughputEnabled {
		zap.S().Debugf("Cleaning up high throughput processor channel (%d)", len(highThroughputProcessorChannel))

		if !kafka2.DrainChannelSimple(highThroughputProcessorChannel, highThroughputPutBackChannel) {
			time.Sleep(other.FiveSeconds)
		}
		if !kafka2.DrainChannelSimple(processValueChannel, highThroughputPutBackChannel) {
			time.Sleep(other.FiveSeconds)
		}
		if !kafka2.DrainChannelSimple(processValueStringChannel, highThroughputPutBackChannel) {

			time.Sleep(other.FiveSeconds)
		}

		time.Sleep(other.OneSecond)

		maxAttempts := 50
		attempt := 0

		for len(highThroughputPutBackChannel) > 0 {
			zap.S().Infof("Waiting for putback channel to empty: %d", len(highThroughputPutBackChannel))
			time.Sleep(other.OneSecond)
			attempt++
			if attempt > maxAttempts {
				zap.S().Errorf("Putback channel is not empty after %d attempts, exiting", maxAttempts)
				break
			}
		}
	}

	kafka2.ShutdownPutback = true

	time.Sleep(other.OneSecond)

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
		commitsPerSecond := (kafka2.KafkaCommits - lastCommits) / sleepS
		messagesPerSecond := (kafka2.KafkaMessages - lastMessages) / sleepS
		putbacksPerSecond := (kafka2.KafkaPutBacks - lastPutbacks) / sleepS
		confirmedPerSecond := (kafka2.KafkaConfirmed - lastConfirmed) / sleepS
		lastCommits = kafka2.KafkaCommits
		lastMessages = kafka2.KafkaMessages
		lastPutbacks = kafka2.KafkaPutBacks
		lastConfirmed = kafka2.KafkaConfirmed

		zap.S().Infof("Performance report"+
			"| Commits: %f, Commits/s: %f"+
			"| Messages: %f, Messages/s: %f"+
			"| PutBacks: %f, PutBacks/s: %f"+
			"| Confirmed: %f, Confirmed/s: %f"+
			"| [HI] Processor queue length: %d"+
			"| [HI] PutBack queue length: %d"+
			"| [HI] Commit queue length: %d"+
			"| Messagecache hitrate %f"+
			"| Dbcache hitrate %f"+
			"| [HT] ProcessValue queue lenght: %d"+
			"| [HT] ProcessValueString queue lenght: %d"+
			"| [HT] Processor queue length: %d"+
			"| [HT] PutBack queue length: %d",
			kafka2.KafkaCommits, commitsPerSecond,
			kafka2.KafkaMessages, messagesPerSecond,
			kafka2.KafkaPutBacks, putbacksPerSecond,
			kafka2.KafkaConfirmed, confirmedPerSecond,
			len(highIntegrityProcessorChannel),
			len(highIntegrityPutBackChannel),
			len(highIntegrityCommitChannel),
			kafka2.Messagecache.HitRate(),
			dbcache.HitRate(),
			len(processValueChannel),
			len(processValueStringChannel),
			len(highThroughputProcessorChannel),
			len(highThroughputPutBackChannel),
		)

		if kafka2.KafkaCommits > math.MaxFloat64/2 || lastCommits > math.MaxFloat64/2 {
			kafka2.KafkaCommits = 0
			lastCommits = 0
			zap.S().Warnf("Resetting commit statistics")
		}

		if kafka2.KafkaMessages > math.MaxFloat64/2 || lastMessages > math.MaxFloat64/2 {
			kafka2.KafkaMessages = 0
			lastMessages = 0
			zap.S().Warnf("Resetting message statistics")
		}

		if kafka2.KafkaPutBacks > math.MaxFloat64/2 || lastPutbacks > math.MaxFloat64/2 {
			kafka2.KafkaPutBacks = 0
			lastPutbacks = 0
			zap.S().Warnf("Resetting putback statistics")
		}

		if kafka2.KafkaConfirmed > math.MaxFloat64/2 || lastConfirmed > math.MaxFloat64/2 {
			kafka2.KafkaConfirmed = 0
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
