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
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var ActivityEnabled bool

var AnomalyEnabled bool

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

	zap.S().Debugf("Setting up Kafka")
	// Read environment variables for Kafka
	KafkaBootstrapServer, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	kafkaSslPassword, err := env.GetAsString("KAFKA_SSL_KEY_PASSWORD", false, "")
	if err != nil {
		zap.S().Error(err)
	}

	allowedMemorySize, err := env.GetAsInt("MEMORY_REQUEST", false, 1073741824)
	if err != nil {
		zap.S().Error(err)
	}
	zap.S().Infof("Allowed memory size is %d", allowedMemorySize)

	// InitCache is initialized with 1Gb of memory for each cache
	internal.InitMessageCache(allowedMemorySize / 4)

	ActivityEnabled, err = env.GetAsBool("ACTIVITY_ENABLED", false, false)
	if err != nil {
		zap.S().Error(err)
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
			zap.S().Fatalf("Error opening kafka tls.key: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Error opening certificate: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Error opening ca.crt: %s", err)
		}
	}
	if ActivityEnabled {
		SetupActivityKafka(
			kafka.ConfigMap{
				"security.protocol":        securityProtocol,
				"ssl.key.location":         "/SSL_certs/kafka/tls.key",
				"ssl.key.password":         kafkaSslPassword,
				"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
				"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
				"bootstrap.servers":        KafkaBootstrapServer,
				"group.id":                 "kafka-state-detector-activity",
				"enable.auto.commit":       true,
				"enable.auto.offset.store": false,
				"auto.offset.reset":        "earliest",
				"metadata.max.age.ms":      180000,
			})

		ActivityProcessorChannel = make(chan *kafka.Message, 100)
		ActivityCommitChannel = make(chan *kafka.Message)
		activityEventChannel := ActivityKafkaProducer.Events()
		activityTopic := "^ia\\.\\w*\\.\\w*\\.\\w*\\.activity$"

		ActivityPutBackChannel = make(chan internal.PutBackChanMsg, 200)
		go internal.StartPutbackProcessor(
			"[AC]",
			ActivityPutBackChannel,
			ActivityKafkaProducer,
			ActivityCommitChannel,
			200)
		go internal.ProcessKafkaQueue(
			"[AC]",
			activityTopic,
			ActivityProcessorChannel,
			ActivityKafkaConsumer,
			ActivityPutBackChannel,
			ShutdownApplicationGraceful)
		go internal.StartCommitProcessor("[AC]", ActivityCommitChannel, ActivityKafkaConsumer)
		go internal.StartEventHandler("[AC]", activityEventChannel, ActivityPutBackChannel)
		go startActivityProcessor()
	}

	AnomalyEnabled, err = env.GetAsBool("ANOMALY_ENABLED", false, false)
	if err != nil {
		zap.S().Error(err)
	}

	if AnomalyEnabled {
		/* #nosec G404 -- This doesn't have to be crypto random */
		SetupAnomalyKafka(
			kafka.ConfigMap{
				"bootstrap.servers":        KafkaBootstrapServer,
				"security.protocol":        securityProtocol,
				"ssl.key.location":         "/SSL_certs/kafka/tls.key",
				"ssl.key.password":         kafkaSslPassword,
				"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
				"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
				"group.id":                 fmt.Sprintf("kafka-state-detector-anomaly-%d", rand.Uint64()),
				"enable.auto.commit":       true,
				"auto.offset.reset":        "earliest",
				"metadata.max.age.ms":      180000,
			})

		AnomalyProcessorChannel = make(chan *kafka.Message, 100)
		AnomalyCommitChannel = make(chan *kafka.Message)
		anomalyEventChannel := AnomalyKafkaProducer.Events()
		anomalyTopic := "^ia\\.\\w*\\.\\w*\\.\\w*\\.activity$"

		AnomalyPutBackChannel = make(chan internal.PutBackChanMsg, 200)
		go internal.StartPutbackProcessor(
			"[AN]",
			AnomalyPutBackChannel,
			AnomalyKafkaProducer,
			ActivityCommitChannel,
			200)
		go internal.ProcessKafkaQueue(
			"[AN]",
			anomalyTopic,
			AnomalyProcessorChannel,
			AnomalyKafkaConsumer,
			AnomalyPutBackChannel,
			ShutdownApplicationGraceful)
		go internal.StartCommitProcessor("[AN]", AnomalyCommitChannel, AnomalyKafkaConsumer)
		go internal.StartEventHandler("[AN]", anomalyEventChannel, AnomalyPutBackChannel)
		go startAnomalyActivityProcessor()
	}

	if !ActivityEnabled && !AnomalyEnabled {
		zap.S().Fatal("No activity or anomaly processing enabled")
	}

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

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
