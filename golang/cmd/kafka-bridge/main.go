package main

import (
	"github.com/heptiolabs/healthcheck"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"math"
	"net/http"

	/* #nosec G108 -- Replace with https://github.com/felixge/fgtrace later*/
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
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	err := json.Unmarshal(data, &r)
	return r, err
}

type TopicMapElement struct {
	Name          string  `json:"name"`
	Topic         string  `json:"topic"`
	SendDirection SendDir `json:"send_direction,omitempty"`
	Bidirectional bool    `json:"bidirectional"`
}

var LocalKafkaBootstrapServers string
var RemoteKafkaBootstrapServers string

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)
	zap.S().Infof("This is kafka-bridge build date: %s", buildtime)

	// pprof
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("localhost:1337", nil)
		if err != nil {
			zap.S().Errorf("Error starting pprof: %s", err)
		}
	}()

	// Prometheus
	metricsPath := "/metrics"
	metricsPort := ":2112"
	zap.S().Debugf("Setting up metrics %s %v", metricsPath, metricsPort)

	http.Handle(metricsPath, promhttp.Handler())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe(metricsPort, nil)
		if err != nil {
			zap.S().Errorf("Error starting metrics: %s", err)
		}
	}()

	// Prometheus
	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

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
		zap.S().Fatal(err)
	}

	GroupIdSuffic := os.Getenv("KAFKA_GROUP_ID_SUFFIX")

	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
		securityProtocol = "ssl"

		_, err := os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatal(err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatal(err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatal(err)
		}
	}
	CreateTopicMapProcessors(topicMap, GroupIdSuffic, securityProtocol)

	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	// It's important to handle both signals, allowing Kafka to shut down gracefully !
	// If this is not possible, it will attempt to rebalance itself, which will increase startup time
	signal.Notify(sigs, syscall.SIGTERM)

	go func() {
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Received SIG %v", sig)

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
	internal.ShuttingDownKafka = true

	zap.S().Infof("Awaiting %d shutdowns", ShutdownsRequired)
	for i := 0; i < 10; i++ {
		if ShutdownsRequired != len(ShutdownChannel) {
			zap.S().Infof("Waiting for %d shutdowns", ShutdownsRequired-len(ShutdownChannel))
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	internal.ShutdownPutback = true

	time.Sleep(1 * time.Second)

	zap.S().Infof("Successful shutdown. Exiting.")

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
		confirmsPerSecond := (internal.KafkaConfirmed - lastConfirmed) / sleepS
		lastCommits = internal.KafkaCommits
		lastMessages = internal.KafkaMessages
		lastPutbacks = internal.KafkaPutBacks
		lastConfirmed = internal.KafkaConfirmed

		zap.S().Infof(
			"Performance report"+
				"| Commits: %f, Commits/s: %f"+
				"| Messages: %f, Messages/s: %f"+
				"| PutBacks: %f, PutBacks/s: %f"+
				"| Confirms: %f, Confirms/s: %f",
			internal.KafkaCommits, commitsPerSecond,
			internal.KafkaMessages, messagesPerSecond,
			internal.KafkaPutBacks, putbacksPerSecond,
			internal.KafkaConfirmed, confirmsPerSecond,
		)

		zap.S().Infof(
			"Cache report"+
				"\nEntry count: %d"+
				"\nHitrate: %f"+
				"\nLookup count: %d",
			messageCache.EntryCount(), messageCache.HitRate(), messageCache.LookupCount(),
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
