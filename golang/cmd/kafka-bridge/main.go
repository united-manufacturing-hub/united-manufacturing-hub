package main

import (
	"encoding/json"
	"fmt"
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
	// Setup logger and set as global
	var logger *zap.Logger
	if os.Getenv("LOGGING_LEVEL") == "DEVELOPMENT" {
		logger, _ = zap.NewDevelopment()
	} else {

		logger, _ = zap.NewProduction()
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is kafka-bridge build date: %s", buildtime)

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
	topicMap, err := UnmarshalTopicMap([]byte(KafkaTopicMap))
	if err != nil {
		panic(fmt.Sprintf("Failed to unmarshal topic map: %v", err))
	}

	LocalKafkaBootstrapServers = os.Getenv("LOCAL_KAFKA_BOOTSTRAP_SERVER")
	RemoteKafkaBootstrapServers = os.Getenv("REMOTE_KAFKA_BOOTSTRAP_SERVER")
	if LocalKafkaBootstrapServers == "" || RemoteKafkaBootstrapServers == "" {
		panic("LocalKafkaBootstrapServers and RemoteKafkaBootstrapServers must be set")
	}

	CreateTopicMapProcessors(topicMap)

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
var ShutdownChannel chan bool
var ShutdownsRequired int

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	ShutdownChannel = make(chan bool, ShutdownsRequired)
	zap.S().Infof("Shutting down application")
	ShuttingDown = true

	for ShutdownsRequired != len(ShutdownChannel) {
		zap.S().Debugf("Waiting for %d shutdowns", ShutdownsRequired-len(ShutdownChannel))
		time.Sleep(time.Second)
	}

	ShutdownPutback = true

	time.Sleep(1 * time.Second)

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

var Commits = float64(0)
var Messages = float64(0)
var PutBacks = float64(0)
var Confirmed = float64(0)

func PerformanceReport() {
	lastCommits := float64(0)
	lastMessages := float64(0)
	lastPutbacks := float64(0)
	lastConfirmed := float64(0)
	sleepS := 10.0
	for !ShuttingDown {
		preExecutionTime := time.Now()
		commitsPerSecond := Commits - lastCommits/sleepS
		messagesPerSecond := Messages - lastMessages/sleepS
		putbacksPerSecond := PutBacks - lastPutbacks/sleepS
		confirmsPerSecond := Confirmed - lastConfirmed/sleepS
		lastCommits = Commits
		lastMessages = Messages
		lastPutbacks = PutBacks
		lastConfirmed = Confirmed

		zap.S().Infof("Performance report"+
			"\nCommits: %f, Commits/s: %f"+
			"\nMessages: %f, Messages/s: %f"+
			"\nPutBacks: %f, PutBacks/s: %f"+
			"\nConfirms: %f, Confirms/s: %f",
			Commits, commitsPerSecond,
			Messages, messagesPerSecond,
			PutBacks, putbacksPerSecond,
			Confirmed, confirmsPerSecond,
		)

		zap.S().Infof("Cache report"+
			"\nEntry count: %d"+
			"\nHitrate: %f"+
			"\nLookup count: %d",
			messageCache.EntryCount(), messageCache.HitRate(), messageCache.LookupCount(),
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

		postExecutionTime := time.Now()
		ExecutionTimeDiff := postExecutionTime.Sub(preExecutionTime).Seconds()
		if ExecutionTimeDiff <= 0 {
			continue
		}
		time.Sleep(time.Second * time.Duration(sleepS-ExecutionTimeDiff))
	}
}
