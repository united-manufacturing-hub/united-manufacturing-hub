package main

import (
	"encoding/json"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	kafka2 "github.com/united-manufacturing-hub/umh-lib/v2/kafka"
	"github.com/united-manufacturing-hub/umh-lib/v2/other"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var buildtime string

type SendDir string

const (
	ToRemote SendDir = "to_remote"
	ToLocal  SendDir = "to_local"
)

type TopicMap []TopicMapElement

func UnmarshalTopicMap(data []byte) (TopicMap, error) {
	var r TopicMap
	err := json.Unmarshal(data, &r)
	return r, err
}

type TopicMapElement struct {
	Name          string  `json:"name"`
	Topic         string  `json:"topic"`
	Bidirectional bool    `json:"bidirectional"`
	SendDirection SendDir `json:"send_direction,omitempty"`
}

var LocalKafkaBootstrapServers string
var RemoteKafkaBootstrapServers string

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
	zap.S().Infof("This is kafka-bridge build date: %s", buildtime)

	// pprof
	go http.ListenAndServe("localhost:1337", nil)

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

	zap.S().Debugf("Starting queue processor")
	KafkaTopicMap := os.Getenv("KAFKA_TOPIC_MAP")
	if KafkaTopicMap == "" {
		zap.S().Fatal("Kafka topic map is not set")
	}
	topicMap, err := UnmarshalTopicMap([]byte(KafkaTopicMap))
	if err != nil {
		zap.S().Fatal("Failed to unmarshal topic map: %v", err)
	}

	LocalKafkaBootstrapServers = os.Getenv("LOCAL_KAFKA_BOOTSTRAP_SERVER")
	RemoteKafkaBootstrapServers = os.Getenv("REMOTE_KAFKA_BOOTSTRAP_SERVER")
	if LocalKafkaBootstrapServers == "" || RemoteKafkaBootstrapServers == "" {
		panic("LocalKafkaBootstrapServers and RemoteKafkaBootstrapServers must be set")
	}

	GroupIdSuffic := os.Getenv("KAFKA_GROUP_ID_SUFFIX")

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
	CreateTopicMapProcessors(topicMap, GroupIdSuffic, securityProtocol)

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
var ShutdownChannel chan bool
var ShutdownsRequired int

// ShutdownApplicationGraceful shutdown the Kafka consumers and producers, then itself.
func ShutdownApplicationGraceful() {
	if ShuttingDown {
		return
	}

	ShutdownChannel = make(chan bool, ShutdownsRequired)
	zap.S().Infof("Shutting down application")
	ShuttingDown = true
	kafka2.ShuttingDownKafka = true

	zap.S().Infof("Awaiting %d shutdowns", ShutdownsRequired)
	for i := 0; i < 10; i++ {
		if ShutdownsRequired != len(ShutdownChannel) {
			zap.S().Infof("Waiting for %d shutdowns", ShutdownsRequired-len(ShutdownChannel))
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	kafka2.ShutdownPutback = true

	time.Sleep(1 * time.Second)

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
		confirmsPerSecond := (kafka2.KafkaConfirmed - lastConfirmed) / sleepS
		lastCommits = kafka2.KafkaCommits
		lastMessages = kafka2.KafkaMessages
		lastPutbacks = kafka2.KafkaPutBacks
		lastConfirmed = kafka2.KafkaConfirmed

		zap.S().Infof("Performance report"+
			"| Commits: %f, Commits/s: %f"+
			"| Messages: %f, Messages/s: %f"+
			"| PutBacks: %f, PutBacks/s: %f"+
			"| Confirms: %f, Confirms/s: %f",
			kafka2.KafkaCommits, commitsPerSecond,
			kafka2.KafkaMessages, messagesPerSecond,
			kafka2.KafkaPutBacks, putbacksPerSecond,
			kafka2.KafkaConfirmed, confirmsPerSecond,
		)

		zap.S().Infof("Cache report"+
			"\nEntry count: %d"+
			"\nHitrate: %f"+
			"\nLookup count: %d",
			messageCache.EntryCount(), messageCache.HitRate(), messageCache.LookupCount(),
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
