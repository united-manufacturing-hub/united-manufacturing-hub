package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/minio/minio-go/v7"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/kafka_helper"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var minioClient *minio.Client

var buildtime string

func main() {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is kafka-to-blob build date: %s", buildtime)

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	KafkaTopic := os.Getenv("KAFKA_LISTEN_TOPIC")
	KafkaBaseTopic := os.Getenv("KAFKA_BASE_TOPIC")

	// Read environment variables for Minio
	MinioUrl := os.Getenv("MINIO_URL")
	MinioAccessKey := os.Getenv("MINIO_ACCESS_KEY")
	MinioSecretKey := os.Getenv("MINIO_SECRET_KEY")
	MinioSecureStr := os.Getenv("MINIO_SECURE")
	MinioSecure := MinioSecureStr == "1" || strings.ToLower(MinioSecureStr) == "true"
	MinioBucketName := os.Getenv("BUCKET_NAME")

	zap.S().Debugf("Setting up Kafka")
	kafka_helper.SetupKafka(kafka.ConfigMap{
		"bootstrap.servers": KafkaBoostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "kafka-to-blob",
	})

	err := kafka_helper.CreateTopicIfNotExists(KafkaBaseTopic)
	if err != nil {
		panic(err)
	}

	zap.S().Debugf("Setting up Minio")
	minioClient = setupMinio(MinioUrl, MinioAccessKey, MinioSecretKey, MinioSecure, MinioBucketName)

	zap.S().Debugf("Start Queue processors")
	go processKafkaQueue(KafkaTopic, MinioBucketName)
	go reconnectMinio()

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
		zap.S().Infof("Recieved SIGTERM", sig)

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

	kafka_helper.CloseKafka()

	time.Sleep(15 * time.Second) // Wait that all data is processed

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
