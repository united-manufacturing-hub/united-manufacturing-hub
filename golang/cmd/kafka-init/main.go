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
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net"
	"os"
	"os/signal"
	"syscall"
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

	// Read environment variables for Kafka
	KafkaBootstrapServer, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("KafkaBoostrapServer: %s", KafkaBootstrapServer)
	// Semicolon separated list of topic to create
	KafkaTopics, err := env.GetAsString("KAFKA_TOPICS", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Infof("KafkaTopics: %s", KafkaTopics)
	kafkaSslPassword, _ := env.GetAsString("KAFKA_SSL_KEY_PASSWORD", false, "")

	zap.S().Debugf("Setting up Kafka")
	securityProtocol := "plaintext"
	useSsl, _ := env.GetAsBool("KAFKA_USE_SSL", false, false)
	if useSsl {
		zap.S().Infof("Using SSL")
		securityProtocol = "ssl"

		_, err := os.Open("/SSL_certs/kafka/tls.key")
		if err != nil {
			zap.S().Fatalf("Error opening key file: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/tls.crt")
		if err != nil {
			zap.S().Fatalf("Error opening certificate: %s", err)
		}
		_, err = os.Open("/SSL_certs/kafka/ca.crt")
		if err != nil {
			zap.S().Fatalf("Error opening ca.crt: %s", err)
		}
	} else {
		zap.S().Infof("Using plaintext")
	}

	timeout := 10 * time.Second
	conn, err := net.DialTimeout("tcp", KafkaBootstrapServer, timeout)
	if err != nil {
		zap.S().Errorf("site unreachable, error: %v", err)
	} else {
		zap.S().Infof("Site reachable, connection: %v", conn)
	}
	defer func(conn net.Conn) {
		err = conn.Close()
		if err != nil {
			zap.S().Errorf("Error closing connection: %s", err)
		}
	}(conn)

	internal.SetupKafka(
		kafka.ConfigMap{
			"security.protocol":        securityProtocol,
			"ssl.key.location":         "/SSL_certs/kafka/tls.key",
			"ssl.key.password":         kafkaSslPassword,
			"ssl.certificate.location": "/SSL_certs/kafka/tls.crt",
			"ssl.ca.location":          "/SSL_certs/kafka/ca.crt",
			"bootstrap.servers":        KafkaBootstrapServer,
			"group.id":                 "kafka-init",
			"metadata.max.age.ms":      180000,
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
		zap.S().Infof("Received SIGTERM", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	ShutdownApplicationGraceful()
}

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")

	internal.CloseKafka()

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
