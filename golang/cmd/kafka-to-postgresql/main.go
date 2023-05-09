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
	"bytes"
	"github.com/Shopify/sarama"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"math"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"text/template"
	"time"
)

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION")
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	dryRun, _ := env.GetAsBool("DRY_RUN", false, false)

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

	// Postgres
	PQHost, _ := env.GetAsString("POSTGRES_HOST", false, "db")
	PQPort, _ := env.GetAsInt("POSTGRES_PORT", false, 5432)
	PQUser, err := env.GetAsString("POSTGRES_USER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	PQPassword, err := env.GetAsString("POSTGRES_PASSWORD", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	PQDBName, err := env.GetAsString("POSTGRES_DATABASE", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	PQSSLMode, err := env.GetAsString("POSTGRES_SSL_MODE", false, "require")

	SetupDB(PQUser, PQPassword, PQDBName, PQHost, PQPort, health, dryRun, PQSSLMode)

	zap.S().Debugf("Setting up Kafka")
	// Read environment variables for Kafka
	KafkaBootstrapServer, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	// Customer Name cannot begin with raw
	HITopic := `^ia\.(([^r.](\d|-|\w)*)|(r[b-z](\d|-|\w)*)|(ra[^w]))\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.((addMaintenanceActivity)|(addOrder)|(addParentToChild)|(addProduct)|(addShift)|(count)|(deleteShiftByAssetIdAndBeginTimestamp)|(deleteShiftById)|(endOrder)|(modifyProducedPieces)|(modifyState)|(productTag)|(productTagString)|(recommendation)|(scrapCount)|(startOrder)|(state)|(uniqueProduct)|(scrapUniqueProduct))$`
	HTTopic := `^ia\.(([^r.](\d|-|\w)*)|(r[b-z](\d|-|\w)*)|(ra[^w]))\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.(process[V|v]alue).*$`

	useSsl, _ := env.GetAsBool("KAFKA_USE_SSL", false, false)
	if useSsl {

		_, err := os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatalf("Error opening Kafka TLS key: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Error opening kafka cert: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Error opening ca.crt: %s", err)
		}
	}

	// Combining enable.auto.commit and enable.auto.offset.store
	// leads to better performance.
	// Processed message now will be stored locally and then automatically committed to Kafka.
	// This still provides the at-least-once guarantee.
	//TODO: Make equivalent configurations for enble.auto.commit: true and enable.auto.offset.store: false (confluent-kafka-go) in Sarama kafka
	SetupHIKafka(
		kafka.NewClientOptions{
			Brokers: []string{
				KafkaBootstrapServer,
			},
			ConsumerName:      "kafka-to-postgresql-hi-processor",
			Partitions:        6,
			ReplicationFactor: 1,
			EnableTLS:         useSsl,
			StartOffset:       sarama.OffsetOldest,
		})

	// HT uses enable.auto.commit=true for increased performance.
	//TODO: Make a equivalent configuration for enble.auto.commit: true (confluent-kafka-go) in Sarama kafka
	SetupHTKafka(
		kafka.NewClientOptions{
			Brokers: []string{
				KafkaBootstrapServer,
			},
			ConsumerName:      "kafka-to-postgresql-ht-processor",
			Partitions:        6,
			ReplicationFactor: 1,
			EnableTLS:         useSsl,
			StartOffset:       sarama.OffsetOldest,
		})

	// KafkaTopicProbeConsumer receives a message when a new topic is created
	SetupKafkaTopicProbeConsumer(
		kafka.NewClientOptions{
			Brokers: []string{
				KafkaBootstrapServer,
			},
			ConsumerName:      "kafka-to-postgresql-topic-probe",
			Partitions:        6,
			ReplicationFactor: 1,
			EnableTLS:         useSsl,
			StartOffset:       sarama.OffsetOldest,
		})

	allowedMemorySize, _ := env.GetAsInt("MEMORY_REQUEST", false, 1073741824)
	zap.S().Infof("Allowed memory size is %d", allowedMemorySize)

	// InitCache is initialized with 1Gb of memory for each cache
	InitCache(allowedMemorySize / 4)
	internal.InitMessageCache(allowedMemorySize / 4)

	zap.S().Debugf("Starting queue processor")

	// Start HI related processors
	zap.S().Debugf("Starting HI queue processor")
	highIntegrityProcessorChannel = make(chan *kafka.Message, 100)
	highIntegrityPutBackChannel = make(chan PutBackProducerChanMsg, 200)
	highIntegrityCommitChannel = make(chan *sarama.ProducerMessage)
	//TODO: Get events channel of the HI producer by using Sarama. How to get producer.input() using wrapper??
	highIntegrityErrorsChannel := HIKafkaClient.Producer.Errors()
	highIntegritySuccessesChannel := HIKafkaClient.Producer.Successes()

	go StartPutbackProcessor(
		"[HI]",
		highIntegrityPutBackChannel,
		HIKafkaClient,
		highIntegrityCommitChannel,
		200)
	go ProcessKafkaQueue(
		"[HI]",
		HITopic,
		highIntegrityProcessorChannel,
		HIKafkaClient,
		highIntegrityPutBackChannel,
		ShutdownApplicationGraceful)
	go StartCommitProcessor("[HI]", highIntegrityCommitChannel, HIKafkaClient, "kafka-to-postgresql-hi-processor")

	go startHighIntegrityQueueProcessor()
	go StartProducerEventHandler("[HI]", highIntegrityErrorsChannel, highIntegritySuccessesChannel, highIntegrityPutBackChannel)
	zap.S().Debugf("Started HI queue processor")

	// Start HT related processors
	zap.S().Debugf("Starting HT queue processor")
	highThroughputProcessorChannel = make(chan *kafka.Message, 1000)
	highThroughputPutBackChannel = make(chan PutBackProducerChanMsg, 200)
	highThroughputErrorsChannel := HIKafkaClient.Producer.Errors()
	highThroughputSuccessesChannel := HIKafkaClient.Producer.Successes()
	// HT has no commit channel, it uses auto commit

	go StartPutbackProcessor("[HT]", highThroughputPutBackChannel, HTKafkaClient, nil, 200)
	go ProcessKafkaQueue(
		"[HT]",
		HTTopic,
		highThroughputProcessorChannel,
		HTKafkaClient,
		highThroughputPutBackChannel,
		nil)

	go startHighThroughputQueueProcessor()
	go StartProducerEventHandler("[HT]", highThroughputErrorsChannel, highThroughputSuccessesChannel, highIntegrityPutBackChannel)

	go startProcessValueQueueAggregator()
	go startProcessValueStringQueueAggregator()
	zap.S().Debugf("Started HT queue processor")

	// Start topic probe processor
	zap.S().Debugf("Starting TP queue processor")
	topicProbeProcessorChannel := make(chan *kafka.Message, 100)

	go ProcessKafkaTopicProbeQueue("[TP]", topicProbeProcessorChannel, nil)
	go StartConsumerEventHandler("[TP]", KafkaTopicProbeConsumer.Consumer.Errors(), KafkaTopicProbeConsumer.GetMessages())

	go StartTopicProbeQueueProcessor(topicProbeProcessorChannel)
	zap.S().Debugf("Started TP queue processor")

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

	// The following code keeps the memory usage low
	debug.SetGCPercent(10)

	// go internal.MemoryLimiter(allowedMemorySize)

	go PerformanceReport()
	select {} // block forever
}

var ShuttingDown bool

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	if ShuttingDown {
		zap.S().Infof("Application is already shutting down")
		// Already shutting down
		return
	}

	zap.S().Infof("Shutting down application")
	ShuttingDown = true

	ShuttingDownKafka = true

	// Important, allows high load processors to finish
	time.Sleep(time.Second * 5)

	zap.S().Infof("Cleaning up high integrity processor channel (%d)", len(highIntegrityProcessorChannel))

	if !DrainChannelSimple(highIntegrityProcessorChannel, highIntegrityPutBackChannel) {

		time.Sleep(internal.FiveSeconds)
	}

	time.Sleep(internal.OneSecond)

	maxAttempts := 50
	attempt := 0

	for len(highIntegrityPutBackChannel) > 0 {
		zap.S().Infof("Waiting for putback channel to empty: %d", len(highIntegrityPutBackChannel))
		time.Sleep(internal.OneSecond)
		attempt++
		if attempt > maxAttempts {
			zap.S().Errorf("Putback channel is not empty after %d attempts, exiting", maxAttempts)
			break
		}
	}

	// This is behind HI to allow a higher chance of a clean shutdown
	zap.S().Infof("Cleaning up high throughput processor channel (%d)", len(highThroughputProcessorChannel))

	if !DrainChannelSimple(highThroughputProcessorChannel, highThroughputPutBackChannel) {
		time.Sleep(internal.FiveSeconds)
	}
	if !DrainChannelSimple(processValueChannel, highThroughputPutBackChannel) {
		time.Sleep(internal.FiveSeconds)
	}
	if !DrainChannelSimple(processValueStringChannel, highThroughputPutBackChannel) {

		time.Sleep(internal.FiveSeconds)
	}

	time.Sleep(internal.OneSecond)

	for len(highThroughputPutBackChannel) > 0 {
		zap.S().Infof("Waiting for putback channel to empty: %d", len(highThroughputPutBackChannel))
		time.Sleep(internal.OneSecond)
		attempt++
		if attempt > maxAttempts {
			zap.S().Errorf("Putback channel is not empty after %d attempts, exiting", maxAttempts)
			break
		}
	}

	ShutdownPutback = true

	time.Sleep(internal.OneSecond)

	CloseHIKafka()

	CloseHTKafka()

	CloseKafkaTopicProbeConsumer()

	ShutdownDB()

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

type reportData struct {
	LastKafkaMessageReceived           string
	ProcessorQueueLength               int
	PutBacksPerSecond                  float64
	PutBackQueueLength                 int
	PutBacks                           float64
	CommitQueueLength                  int
	Confirmed                          float64
	ConfirmedPerSecond                 float64
	MessagecacheHitRate                float64
	MessagesPerSecond                  float64
	Messages                           float64
	Commits                            float64
	DbcacheHitRate                     float64
	ProcessValueQueueLength            int
	ProcessValueStringQueueLength      int
	HighThroughputProcessorQueueLength int
	HighThroughputPutBackQueueLength   int
	CommitsPerSecond                   float64
}

const reportTemplate = `Performance report
| Commits: {{.Commits}}, Commits/s: {{.CommitsPerSecond}}
| Messages: {{.Messages}}, Messages/s: {{.MessagesPerSecond}}
| PutBacks: {{.PutBacks}}, PutBacks/s: {{.PutBacksPerSecond}}
| Confirmed: {{.Confirmed}}, Confirmed/s: {{.ConfirmedPerSecond}}
| [HI] Processor queue length: {{.ProcessorQueueLength}}
| [HI] PutBack queue length: {{.PutBackQueueLength}}
| [HI] Commit queue length: {{.CommitQueueLength}}
| Messagecache hitrate {{.MessagecacheHitRate}}
| Dbcache hitrate {{.DbcacheHitRate}}
| [HT] ProcessValue queue length: {{.ProcessValueQueueLength}}
| [HT] ProcessValueString queue length: {{.ProcessValueStringQueueLength}}
| [HT] Processor queue length: {{.HighThroughputProcessorQueueLength}}
| [HT] PutBack queue length: {{.HighThroughputPutBackQueueLength}}
| Last Kafka message received: {{.LastKafkaMessageReceived}}`

func PerformanceReport() {
	lastCommits := float64(0)
	lastMessages := float64(0)
	lastPutbacks := float64(0)
	lastConfirmed := float64(0)
	sleepS := 10.0

	t, err := template.New("report").Parse(reportTemplate)
	if err != nil {
		zap.S().Errorf("Error parsing template: %s", err.Error())
		ShutdownApplicationGraceful()
		return
	}

	for !ShuttingDown {
		// Prevent data-race with channel creation
		if highIntegrityProcessorChannel == nil ||
			highIntegrityPutBackChannel == nil ||
			highIntegrityCommitChannel == nil ||
			processValueChannel == nil ||
			processValueStringChannel == nil ||
			highThroughputProcessorChannel == nil ||
			highThroughputPutBackChannel == nil {
			time.Sleep(time.Second * 1)
			continue
		}

		preExecutionTime := time.Now()
		commitsPerSecond := (KafkaCommits - lastCommits) / sleepS
		messagesPerSecond := (KafkaMessages - lastMessages) / sleepS
		putbacksPerSecond := (KafkaPutBacks - lastPutbacks) / sleepS
		confirmedPerSecond := (KafkaConfirmed - lastConfirmed) / sleepS
		lastCommits = KafkaCommits
		lastMessages = KafkaMessages
		lastPutbacks = KafkaPutBacks
		lastConfirmed = KafkaConfirmed

		data := reportData{
			Commits:                            KafkaCommits,
			CommitsPerSecond:                   commitsPerSecond,
			Messages:                           KafkaMessages,
			MessagesPerSecond:                  messagesPerSecond,
			PutBacks:                           KafkaPutBacks,
			PutBacksPerSecond:                  putbacksPerSecond,
			Confirmed:                          KafkaConfirmed,
			ConfirmedPerSecond:                 confirmedPerSecond,
			ProcessorQueueLength:               len(highIntegrityProcessorChannel),
			PutBackQueueLength:                 len(highIntegrityPutBackChannel),
			CommitQueueLength:                  len(highIntegrityCommitChannel),
			MessagecacheHitRate:                internal.Messagecache.HitRate(),
			DbcacheHitRate:                     dbcache.HitRate(),
			ProcessValueQueueLength:            len(processValueChannel),
			ProcessValueStringQueueLength:      len(processValueStringChannel),
			HighThroughputProcessorQueueLength: len(highThroughputProcessorChannel),
			HighThroughputPutBackQueueLength:   len(highThroughputPutBackChannel),
			LastKafkaMessageReceived:           internal.LastKafkaMessageReceived.Format(time.RFC3339),
		}
		var report bytes.Buffer
		err := t.Execute(&report, data)
		if err != nil {
			zap.S().Errorf("Error executing performance report template: %v", err)
			return
		}
		zap.S().Infof("Performance report: %s", report.String())

		if KafkaCommits > math.MaxFloat64/2 || lastCommits > math.MaxFloat64/2 {
			KafkaCommits = 0
			lastCommits = 0
			zap.S().Warnf("Resetting commit statistics")
		}

		if KafkaMessages > math.MaxFloat64/2 || lastMessages > math.MaxFloat64/2 {
			KafkaMessages = 0
			lastMessages = 0
			zap.S().Warnf("Resetting message statistics")
		}

		if KafkaPutBacks > math.MaxFloat64/2 || lastPutbacks > math.MaxFloat64/2 {
			KafkaPutBacks = 0
			lastPutbacks = 0
			zap.S().Warnf("Resetting putback statistics")
		}

		if KafkaConfirmed > math.MaxFloat64/2 || lastConfirmed > math.MaxFloat64/2 {
			KafkaConfirmed = 0
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
