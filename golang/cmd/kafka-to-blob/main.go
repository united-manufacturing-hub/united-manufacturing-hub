package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/minio/minio-go/v7"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var minioClient *minio.Client

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

	zap.S().Infof("This is kafka-to-blob build date: %s", buildtime)

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

	// Read environment variables for Kafka
	KafkaBoostrapServer, KafkaBoostrapServerEnvSet := os.LookupEnv("KAFKA_BOOTSTRAP_SERVER")
	if !KafkaBoostrapServerEnvSet {
		zap.S().Fatal("Kafka Boostrap Server (KAFKA_BOOTSTRAP_SERVER) must be set")
	}
	KafkaTopic, KafkaTopicEnvSet := os.LookupEnv("KAFKA_LISTEN_TOPIC")
	if !KafkaTopicEnvSet {
		zap.S().Fatal("Kafka Topic (KAFKA_LISTEN_TOPIC) must be set")
	}
	KafkaBaseTopic, KafkaBaseTopicEnvSet := os.LookupEnv("KAFKA_BASE_TOPIC")
	if !KafkaBaseTopicEnvSet {
		zap.S().Fatal("Kafka Base Topic (KAFKA_BASE_TOPIC) must be set")
	}

	// Read environment variables for Minio
	MinioUrl, MinioUrlEnvSet := os.LookupEnv("MINIO_URL")
	if !MinioUrlEnvSet {
		zap.S().Fatal("Minio URL (MINIO_URL) must be set")
	}
	MinioAccessKey, MinioAccessKeyEnvSet := os.LookupEnv("MINIO_ACCESS_KEY")
	if !MinioAccessKeyEnvSet {
		zap.S().Fatal("Minio Acces Key (MINIO_ACCESS_KEY) must be set")
	}
	MinioSecretKey, MinioSecretKeyEnvSet := os.LookupEnv("MINIO_SECRET_KEY")
	if !MinioSecretKeyEnvSet {
		zap.S().Fatal("Minio Secret Key (MINIO_SECRET_KEY) must be set")
	}
	MinioSecureStr, MinioSecureStrEnvSet := os.LookupEnv("MINIO_SECURE")
	if !MinioSecureStrEnvSet {
		zap.S().Fatal("Minio SecureStr (MINIO_SECURE) must be set")
	}
	MinioSecure := MinioSecureStr == "1" || strings.EqualFold(MinioSecureStr, "true")
	MinioBucketName, MinioBucketNameEnvSet := os.LookupEnv("BUCKET_NAME")
	if !MinioBucketNameEnvSet {
		zap.S().Fatal("Minio Bucket name (BUCKET_NAME) must be set")
	}

	zap.S().Debugf("Setting up Kafka")
	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
		securityProtocol = "ssl"

		_, err := os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatalf("Error opening Kafka TLS key: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Error opening certificate: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Error opening ca.crt: %v", err)
		}
	}
	internal.SetupKafka(
		kafka.ConfigMap{
			"bootstrap.servers":        KafkaBoostrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"group.id":                 "kafka-to-blob",
			"metadata.max.age.ms":      180000,
		})

	// KafkaTopicProbeConsumer receives a message when a new topic is created
	internal.SetupKafkaTopicProbeConsumer(
		kafka.ConfigMap{
			"bootstrap.servers":        KafkaBoostrapServer,
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"group.id":                 "kafka-to-blob-topic-probe",
			"enable.auto.commit":       true,
			"auto.offset.reset":        "earliest",
			// "debug":                    "security,broker",
			"topic.metadata.refresh.interval.ms": "30000",
		})

	err := internal.CreateTopicIfNotExists(KafkaBaseTopic)
	if err != nil {
		zap.S().Fatalf("Failed to create topic %s: %s", KafkaBaseTopic, err)
	}

	zap.S().Debugf("Setting up Minio")
	minioClient = setupMinio(MinioUrl, MinioAccessKey, MinioSecretKey, MinioSecure, MinioBucketName)

	zap.S().Debugf("Start Queue processors")
	go processKafkaQueue(KafkaTopic, MinioBucketName)
	go reconnectMinio()

	// Start topic probe processor
	zap.S().Debugf("Starting TP queue processor")
	topicProbeProcessorChannel := make(chan *kafka.Message, 100)

	go internal.ProcessKafkaTopicProbeQueue("[TP]", topicProbeProcessorChannel, nil)
	go internal.StartEventHandler("[TP]", internal.KafkaTopicProbeConsumer.Events(), nil)

	go internal.StartTopicProbeQueueProcessor(topicProbeProcessorChannel)
	zap.S().Debugf("Started TP queue processor")

	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	go func() {
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.
		//
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Received SIGTERM", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {} // block forever
}

var ShuttingDown bool

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	ShuttingDown = true

	internal.CloseKafka()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
