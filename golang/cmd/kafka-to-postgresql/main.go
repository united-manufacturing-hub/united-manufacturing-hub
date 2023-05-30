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
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/umh-utils/parse"
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
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	dryRun, err := env.GetAsBool("DRY_RUN", false, false)
	if err != nil {
		zap.S().Error(err)
	}

	// Prometheus
	metricsPath := "/metrics"
	metricsPort := ":2112"
	zap.S().Debugf("Setting up metrics %s %v", metricsPath, metricsPort)

	http.Handle(metricsPath, promhttp.Handler())
	go func() {
		/* #nosec G114 */
		err = http.ListenAndServe(metricsPort, nil)
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
		err = http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

	// Postgres
	PQHost, err := env.GetAsString("POSTGRES_HOST", false, "db")
	if err != nil {
		zap.S().Error(err)
	}
	PQPort, err := env.GetAsInt("POSTGRES_PORT", false, 5432)
	if err != nil {
		zap.S().Error(err)
	}
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
	if err != nil {
		zap.S().Error(err)
	}

	SetupDB(PQUser, PQPassword, PQDBName, PQHost, PQPort, health, dryRun, PQSSLMode)

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

	// Customer Name cannot begin with raw
	HITopic := `^ia\.(([^r.](\d|-|\w)*)|(r[b-z](\d|-|\w)*)|(ra[^w]))\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.((addMaintenanceActivity)|(addOrder)|(addParentToChild)|(addProduct)|(addShift)|(count)|(deleteShiftByAssetIdAndBeginTimestamp)|(deleteShiftById)|(endOrder)|(modifyProducedPieces)|(modifyState)|(productTag)|(productTagString)|(recommendation)|(scrapCount)|(startOrder)|(state)|(uniqueProduct)|(scrapUniqueProduct))$`
	HTTopic := `^ia\.(([^r.](\d|-|\w)*)|(r[b-z](\d|-|\w)*)|(ra[^w]))\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.(process[V|v]alue).*$`

	securityProtocol := "plaintext"
	useSsl, err := env.GetAsBool("KAFKA_USE_SSL", false, false)
	if err != nil {
		zap.S().Error(err)
	}
	if useSsl {
		securityProtocol = "ssl"

		_, err = os.Open("/SSL_certs/kafka/tls.key")
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
	SetupHIKafka(
		kafka.ConfigMap{
			"bootstrap.servers":        KafkaBootstrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         kafkaSslPassword,
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"group.id":                 "kafka-to-postgresql-hi-processor",
			"enable.auto.commit":       true,
			"enable.auto.offset.store": false,
			"auto.offset.reset":        "earliest",
			// "debug":                    "security,broker",
			"topic.metadata.refresh.interval.ms": "30000",
			"metadata.max.age.ms":                180000,
		})

	// HT uses enable.auto.commit=true for increased performance.
	SetupHTKafka(
		kafka.ConfigMap{
			"bootstrap.servers":        KafkaBootstrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         kafkaSslPassword,
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"group.id":                 "kafka-to-postgresql-ht-processor",
			"enable.auto.commit":       true,
			"auto.offset.reset":        "earliest",
			// "debug":                    "security,broker",
			"topic.metadata.refresh.interval.ms": "30000",
			"metadata.max.age.ms":                180000,
		})

	// KafkaTopicProbeConsumer receives a message when a new topic is created
	internal.SetupKafkaTopicProbeConsumer(
		kafka.ConfigMap{
			"bootstrap.servers":        KafkaBootstrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         kafkaSslPassword,
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"group.id":                 "kafka-to-postgresql-topic-probe",
			"enable.auto.commit":       true,
			"auto.offset.reset":        "earliest",
			// "debug":                    "security,broker",
			"topic.metadata.refresh.interval.ms": "30000",
		})

	allowedMemorySize, err := env.GetAsString("MEMORY_REQUEST", false, "50Mi")
	if err != nil {
		zap.S().Error(err)
	}
	allowedMemorySizeInt, err := parse.Quantity(allowedMemorySize)
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("Allowed memory size is %d Bytes", allowedMemorySizeInt)

	// InitCache is initialized with 1Gb of memory for each cache
	InitCache(allowedMemorySizeInt / 4)
	internal.InitMessageCache(allowedMemorySizeInt / 4)

	zap.S().Debugf("Starting queue processor")

	// Start HI related processors
	zap.S().Debugf("Starting HI queue processor")
	highIntegrityProcessorChannel = make(chan *kafka.Message, 100)
	highIntegrityPutBackChannel = make(chan internal.PutBackChanMsg, 200)
	highIntegrityCommitChannel = make(chan *kafka.Message)
	highIntegrityEventChannel := HIKafkaProducer.Events()

	go internal.StartPutbackProcessor(
		"[HI]",
		highIntegrityPutBackChannel,
		HIKafkaProducer,
		highIntegrityCommitChannel,
		200)
	go internal.ProcessKafkaQueue(
		"[HI]",
		HITopic,
		highIntegrityProcessorChannel,
		HIKafkaConsumer,
		highIntegrityPutBackChannel,
		ShutdownApplicationGraceful)
	go internal.StartCommitProcessor("[HI]", highIntegrityCommitChannel, HIKafkaConsumer)

	go startHighIntegrityQueueProcessor()
	go internal.StartEventHandler("[HI]", highIntegrityEventChannel, highIntegrityPutBackChannel)
	zap.S().Debugf("Started HI queue processor")

	// Start HT related processors
	zap.S().Debugf("Starting HT queue processor")
	highThroughputProcessorChannel = make(chan *kafka.Message, 1000)
	highThroughputPutBackChannel = make(chan internal.PutBackChanMsg, 200)
	highThroughputEventChannel := HIKafkaProducer.Events()
	// HT has no commit channel, it uses auto commit

	go internal.StartPutbackProcessor("[HT]", highThroughputPutBackChannel, HTKafkaProducer, nil, 200)
	go internal.ProcessKafkaQueue(
		"[HT]",
		HTTopic,
		highThroughputProcessorChannel,
		HTKafkaConsumer,
		highThroughputPutBackChannel,
		nil)

	go startHighThroughputQueueProcessor()
	go internal.StartEventHandler("[HT]", highThroughputEventChannel, highIntegrityPutBackChannel)

	go startProcessValueQueueAggregator()
	go startProcessValueStringQueueAggregator()
	zap.S().Debugf("Started HT queue processor")

	// Start topic probe processor
	zap.S().Debugf("Starting TP queue processor")
	topicProbeProcessorChannel := make(chan *kafka.Message, 100)

	go internal.ProcessKafkaTopicProbeQueue("[TP]", topicProbeProcessorChannel, nil)
	go internal.StartEventHandler("[TP]", internal.KafkaTopicProbeConsumer.Events(), nil)

	go internal.StartTopicProbeQueueProcessor(topicProbeProcessorChannel)
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

	internal.ShuttingDownKafka = true

	// Important, allows high load processors to finish
	time.Sleep(time.Second * 5)

	zap.S().Infof("Cleaning up high integrity processor channel (%d)", len(highIntegrityProcessorChannel))

	if !internal.DrainChannelSimple(highIntegrityProcessorChannel, highIntegrityPutBackChannel) {

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

	if !internal.DrainChannelSimple(highThroughputProcessorChannel, highThroughputPutBackChannel) {
		time.Sleep(internal.FiveSeconds)
	}
	if !internal.DrainChannelSimple(processValueChannel, highThroughputPutBackChannel) {
		time.Sleep(internal.FiveSeconds)
	}
	if !internal.DrainChannelSimple(processValueStringChannel, highThroughputPutBackChannel) {

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

	internal.ShutdownPutback = true

	time.Sleep(internal.OneSecond)

	CloseHIKafka()

	CloseHTKafka()

	internal.CloseKafkaTopicProbeConsumer()

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
		commitsPerSecond := (internal.KafkaCommits - lastCommits) / sleepS
		messagesPerSecond := (internal.KafkaMessages - lastMessages) / sleepS
		putbacksPerSecond := (internal.KafkaPutBacks - lastPutbacks) / sleepS
		confirmedPerSecond := (internal.KafkaConfirmed - lastConfirmed) / sleepS
		lastCommits = internal.KafkaCommits
		lastMessages = internal.KafkaMessages
		lastPutbacks = internal.KafkaPutBacks
		lastConfirmed = internal.KafkaConfirmed

		data := reportData{
			Commits:                            internal.KafkaCommits,
			CommitsPerSecond:                   commitsPerSecond,
			Messages:                           internal.KafkaMessages,
			MessagesPerSecond:                  messagesPerSecond,
			PutBacks:                           internal.KafkaPutBacks,
			PutBacksPerSecond:                  putbacksPerSecond,
			Confirmed:                          internal.KafkaConfirmed,
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
