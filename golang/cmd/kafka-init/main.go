package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var buildtime string

func main() {
	encoderConfig := ecszap.NewDefaultEncoderConfig()
	var core zapcore.Core
	core = ecszap.NewCore(encoderConfig, os.Stdout, zap.DebugLevel)
	logger := zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(logger)
	defer logger.Sync()
	zap.S().Infof("This is kafka-init build date: %s", buildtime)

	// pprof
	go http.ListenAndServe("localhost:1337", nil)

	// Read environment variables for Kafka
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	zap.S().Infof("KafkaBoostrapServer: %s", KafkaBoostrapServer)
	// Semicolon seperated list of topic to create
	KafkaTopics := os.Getenv("KAFKA_TOPICS")
	zap.S().Infof("KafkaTopics: %s", KafkaTopics)

	zap.S().Debugf("Setting up Kafka")
	securityProtocol := "plaintext"
	if internal.EnvIsTrue("KAFKA_USE_SSL") {
		zap.S().Infof("Using SSL")
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
	} else {
		zap.S().Infof("Using plaintext")
	}

	timeout := 10 * time.Second
	conn, err := net.DialTimeout("tcp", KafkaBoostrapServer, timeout)
	if err != nil {
		zap.S().Errorf("site unreachable, error: %v", err)
	} else {
		log.Printf("Site reachable, connection: %v", conn)
	}
	defer conn.Close()

	internal.SetupKafka(kafka.ConfigMap{
		"security.protocol":        securityProtocol,
		"ssl.key.location":         "/SSL_certs/tls.key",
		"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
		"ssl.certificate.location": "/SSL_certs/tls.crt",
		"ssl.ca.location":          "/SSL_certs/ca.crt",
		"bootstrap.servers":        KafkaBoostrapServer,
		"group.id":                 "kafka-init",
	})

	initKafkaTopics(KafkaTopics)

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

	ShutdownApplicationGraceful()
}

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")

	internal.CloseKafka()

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
