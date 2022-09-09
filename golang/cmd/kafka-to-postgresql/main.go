package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	r "k8s.io/apimachinery/pkg/api/resource"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
)

var buildtime string

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

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
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOTSTRAP_SERVER")
	if KafkaBoostrapServer == "" {
		panic("KAFKA_BOOTSTRAP_SERVER not set")
	}
	// Customer Name cannot begin with raw
	HITopic := `^ia\.(([^r.](\d|-|\w)*)|(r[b-z](\d|-|\w)*)|(ra[^w]))\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.((addMaintenanceActivity)|(addOrder)|(addParentToChild)|(addProduct)|(addShift)|(count)|(deleteShiftByAssetIdAndBeginTimestamp)|(deleteShiftById)|(endOrder)|(modifyProducedPieces)|(modifyState)|(productTag)|(productTagString)|(recommendation)|(scrapCount)|(startOrder)|(state)|(uniqueProduct)|(scrapUniqueProduct))$`
	HTTopic := `^ia\.(([^r.](\d|-|\w)*)|(r[b-z](\d|-|\w)*)|(ra[^w]))\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.(process[V|v]alue).*$`

	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
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
		"topic.metadata.refresh.interval.ms": "30000",
	})

	// HT uses enable.auto.commit=true for increased performance.
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
		"topic.metadata.refresh.interval.ms": "30000",
	})

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
	internal.InitMessageCache(allowedMemorySize / 4)

	zap.S().Debugf("Starting queue processor")

	// Start HI related processors
	zap.S().Debugf("Starting HI queue processor")
	highIntegrityProcessorChannel = make(chan *kafka.Message, 100)
	highIntegrityPutBackChannel = make(chan internal.PutBackChanMsg, 200)
	highIntegrityCommitChannel = make(chan *kafka.Message)
	highIntegrityEventChannel := HIKafkaProducer.Events()

	go internal.StartPutbackProcessor("[HI]", highIntegrityPutBackChannel, HIKafkaProducer, highIntegrityCommitChannel, 200)
	go internal.ProcessKafkaQueue("[HI]", HITopic, highIntegrityProcessorChannel, HIKafkaConsumer, highIntegrityPutBackChannel, ShutdownApplicationGraceful)
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
	go internal.ProcessKafkaQueue("[HT]", HTTopic, highThroughputProcessorChannel, HTKafkaConsumer, highThroughputPutBackChannel, nil)

	go startHighThroughputQueueProcessor()
	go internal.StartEventHandler("[HI]", highThroughputEventChannel, highIntegrityPutBackChannel)

	go startProcessValueQueueAggregator()
	go startProcessValueStringQueueAggregator()
	zap.S().Debugf("Started HT queue processor")

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

	//go internal.MemoryLimiter(allowedMemorySize)

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

	internal.ShuttingDownKafka = true

	// Important, allows high load processors to finish
	time.Sleep(time.Second * 5)

	zap.S().Debugf("Cleaning up high integrity processor channel (%d)", len(highIntegrityProcessorChannel))

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
	zap.S().Debugf("Cleaning up high throughput processor channel (%d)", len(highThroughputProcessorChannel))

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
		commitsPerSecond := (internal.KafkaCommits - lastCommits) / sleepS
		messagesPerSecond := (internal.KafkaMessages - lastMessages) / sleepS
		putbacksPerSecond := (internal.KafkaPutBacks - lastPutbacks) / sleepS
		confirmedPerSecond := (internal.KafkaConfirmed - lastConfirmed) / sleepS
		lastCommits = internal.KafkaCommits
		lastMessages = internal.KafkaMessages
		lastPutbacks = internal.KafkaPutBacks
		lastConfirmed = internal.KafkaConfirmed

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
			internal.KafkaCommits, commitsPerSecond,
			internal.KafkaMessages, messagesPerSecond,
			internal.KafkaPutBacks, putbacksPerSecond,
			internal.KafkaConfirmed, confirmedPerSecond,
			len(highIntegrityProcessorChannel),
			len(highIntegrityPutBackChannel),
			len(highIntegrityCommitChannel),
			internal.Messagecache.HitRate(),
			dbcache.HitRate(),
			len(processValueChannel),
			len(processValueStringChannel),
			len(highThroughputProcessorChannel),
			len(highThroughputPutBackChannel),
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
