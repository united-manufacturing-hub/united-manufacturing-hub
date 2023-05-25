// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type SendDir string

const (
	ToRemote SendDir = "to_remote"
	ToLocal  SendDir = "to_local"
)

type TopicMap []TopicMapElement

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
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

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
	var topicMap TopicMap
	err := env.GetAsType("KAFKA_TOPIC_MAP", &topicMap, true, TopicMap{})
	if err != nil {
		zap.S().Fatal(err)
	}

	LocalKafkaBootstrapServers, err = env.GetAsString("LOCAL_KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	RemoteKafkaBootstrapServers, err = env.GetAsString("REMOTE_KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	GroupIdSuffix, err := env.GetAsString("GROUP_ID_SUFFIX", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	securityProtocol := "plaintext"
	useSsl, err := env.GetAsBool("KAFKA_USE_SSL", false, false)
	if err != nil {
		zap.S().Error(err)
	}
	if useSsl {
		securityProtocol = "ssl"

		_, err = os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatalf("Failed to open kafka tls.key: %v", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Error opening certificate: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Error opening CA certificate: %s", err)
		}
	}
	CreateTopicMapProcessors(topicMap, GroupIdSuffix, securityProtocol)

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
