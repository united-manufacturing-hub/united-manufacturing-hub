package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
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
	KafkaTopic := os.Getenv("KAFKA_LISTEN_TOPIC")
	if KafkaTopic == "" {
		panic("KAFKA_LISTEN_TOPIC not set")
	}

	internal.SetupKafka(kafka.ConfigMap{
		"bootstrap.servers":  KafkaBoostrapServer,
		"security.protocol":  "plaintext",
		"group.id":           "kafka-to-postgresql",
		"enable.auto.commit": true,
		"auto.offset.reset":  "earliest",
	})

	// 1GB, 1GB
	InitCache(1073741824, 1073741824)

	zap.S().Debugf("Starting queue processor")
	// Reduce this number, if pg throws sql: statement is closed !!!
	processors := 20

	processorChannel = make(chan *kafka.Message, processors)
	putBackChannel = make(chan PutBackChan, 200)

	go startPutbackProcessor()
	go processKafkaQueue(KafkaTopic)

	for i := 0; i < processors; i++ {
		go queueProcessor(i)
	}

	go countProcessor()

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

	// Wait if there are any CountMessagesToCommitLater in the queue

	zap.S().Debugf("Cleaning up processor channel (%d)", len(processorChannel))
	if !DrainChannel(processorChannel) {
		time.Sleep(5 * time.Second)
	}
	if !DrainChannel(countChannel) {
		time.Sleep(5 * time.Second)
	}

	time.Sleep(1 * time.Second)

	for len(putBackChannel) > 0 {
		zap.S().Infof("Waiting for putback channel to empty: %d", len(putBackChannel))
		time.Sleep(1 * time.Second)
	}
	ShutdownPutback = true

	time.Sleep(1 * time.Second)

	internal.CloseKafka()

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
var InvalidMessages = 0

func PerformanceReport() {
	lastCommits := 0
	lastMessages := 0
	lastPutbacks := 0
	lastInvalids := 0
	sleepS := 10
	for !ShuttingDown {
		commitsPerSecond := (Commits - lastCommits) / sleepS
		messagesPerSecond := (Messages - lastMessages) / sleepS
		putbacksPerSecond := (PutBacks - lastPutbacks) / sleepS
		invalidsPerSecond := (InvalidMessages - lastInvalids) / sleepS
		lastCommits = Commits
		lastMessages = Messages
		lastPutbacks = PutBacks
		lastInvalids = InvalidMessages

		zap.S().Infof("Performance report"+
			"\nCommits: %d, Commits/s: %d"+
			"\nMessages: %d, Messages/s: %d"+
			"\nPutBacks: %d, PutBacks/s: %d"+
			"\nInvalids: %d, Invalids/s: %d"+
			"\nProcessor queue length: %d"+
			"\nPutBack queue length: %d"+
			"\nCount queue length: %d"+
			"\nMessagecache hitrate %f"+
			"\nDbcache hitrate %f"+
			"\nCount messages commitable: %d", Commits, commitsPerSecond, Messages, messagesPerSecond, PutBacks, putbacksPerSecond, InvalidMessages, invalidsPerSecond, len(processorChannel), len(putBackChannel), len(countChannel), messagecache.HitRate(), dbcache.HitRate(), CountMessagesToCommitLater)

		time.Sleep(time.Second * time.Duration(sleepS))
	}
}
