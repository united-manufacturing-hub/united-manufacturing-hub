package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	r "k8s.io/apimachinery/pkg/api/resource"
	"math/rand"
	"net/http"

	/* #nosec G108 -- Replace with https://github.com/felixge/fgtrace later*/
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var buildtime string

var ActivityEnabled bool

var AnomalyEnabled bool

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			zap.S().Fatalf("Error: %s", err)
		}
	}(log)

	zap.S().Infof("This is kafka-state-detector build date: %s", buildtime)

	// pprof
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("localhost:1337", nil)
		if err != nil {
			zap.S().Errorf("Error starting pprof: %s", err)
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

	zap.S().Debugf("Setting up Kafka")
	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOTSTRAP_SERVER")
	if KafkaBoostrapServer == "" {
		zap.S().Fatal("KAFKA_BOOTSTRAP_SERVER not set")
	}

	allowedMemorySize := 1073741824 // 1GB
	if os.Getenv("MEMORY_REQUEST") != "" {
		memoryRequest := r.MustParse(os.Getenv("MEMORY_REQUEST"))
		i, b := memoryRequest.AsInt64()
		if b {
			allowedMemorySize = int(i) // truncated !
		}
	}
	zap.S().Infof("Allowed memory size is %d", allowedMemorySize)

	// InitCache is initialized with 1Gb of memory for each cache
	internal.InitMessageCache(allowedMemorySize / 4)

	ActivityEnabled = os.Getenv("ACTIVITY_ENABLED") == "true"

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
	if ActivityEnabled {
		SetupActivityKafka(
			kafka.ConfigMap{
				"security.protocol":        securityProtocol,
				"ssl.key.location":         "/SSL_certs/kafka/tls.key",
				"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
				"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
				"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
				"bootstrap.servers":        KafkaBoostrapServer,
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
	AnomalyEnabled = os.Getenv("ANOMALY_ENABLED") == "true"

	if AnomalyEnabled {
		/* #nosec G404 -- This doesn't have to be crypto random */
		SetupAnomalyKafka(
			kafka.ConfigMap{
				"bootstrap.servers":        KafkaBoostrapServer,
				"security.protocol":        securityProtocol,
				"ssl.key.location":         "/SSL_certs/kafka/tls.key",
				"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
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
