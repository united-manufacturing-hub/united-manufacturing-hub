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
	"net/http"
	"strings"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func main() {
	var err error
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err = logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	serialNumber, err := env.GetAsString("SERIAL_NUMBER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	brokerA, err := env.GetAsString("BROKER_A", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	brokerB, err := env.GetAsString("BROKER_B", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	topic, err := env.GetAsString("TOPIC", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	split, err := env.GetAsInt("SPLIT", false, -1)
	if err != nil {
		zap.S().Error(err)
	}
	if split != -1 && split < 3 {
		zap.S().Fatalf("SPLIT must be at least 3. got: %d", split)
	}

	zap.S().Debug("Starting healthcheck")
	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

	var clientA, clientB client
	clientA, err = newClient(brokerA, topic, serialNumber)
	if err != nil {
		zap.S().Fatalf("failed to create client: %s", err)
	}
	clientB, err = newClient(brokerB, topic, serialNumber)
	if err != nil {
		zap.S().Fatalf("failed to create client: %s", err)
	}

	gs := internal.NewGracefulShutdown(func() error {
		zap.S().Info("shutting down")
		var err error
		err = clientA.shutdown()
		if err != nil {
			return err
		}
		err = clientB.shutdown()
		if err != nil {
			return err
		}
		return nil
	})

	msgChanLen, err := env.GetAsInt("MSG_CHANNEL_LENGTH", false, 100)
	if err != nil {
		zap.S().Error(err)
	}
	var msgChan = make(chan kafka.Message, msgChanLen)
	var commitChan = make(chan *kafka.Message, msgChanLen)

	zap.S().Info("starting clients")
	clientA.startConsuming(msgChan, commitChan)
	clientB.startProducing(msgChan, commitChan, split)
	reportStats(msgChan, clientA, clientB, gs)
}

// reportStats logs the number of messages sent and received every 10 seconds. It also shuts down the application if no messages are sent or received for 3 minutes.
func reportStats(msgChan chan kafka.Message, consumerClient, producerClient client, gs internal.GracefulShutdownHandler) {
	sent, _, _, _ := producerClient.getProducerStats()
	recv, _, _, _ := consumerClient.getConsumerStats()

	ticker := time.NewTicker(10 * time.Second)
	shutdownTimer := time.NewTimer(3 * time.Minute)
	for {
		select {
		case <-ticker.C:
			var newSent, newRecv uint64
			newSent, newSentInvalidTopic, newSentInvalidMessage, newSentSkipped := producerClient.getProducerStats()
			newRecv, newRecvInvalidTopic, newRecvInvalidMessage, newRecvSkipped := consumerClient.getConsumerStats()

			sentPerSecond := (newSent - sent) / 10
			recvPerSecond := (newRecv - recv) / 10

			zap.S().Infof("Received: %d (%d/s) Invalid Topic: %d Invalid Message: %d Skipped: %d | Sent: %d (%d/s) Invalid Topic: %d Invalid Message: %d Skipped: %d | Lag: %d",
				newRecv, recvPerSecond,
				newRecvInvalidTopic, newRecvInvalidMessage, newRecvSkipped,
				newSent, sentPerSecond,
				newSentInvalidTopic, newSentInvalidMessage, newSentSkipped,
				len(msgChan))

			if newSent != sent && newRecv != recv {
				shutdownTimer.Reset(3 * time.Minute)
				sent, recv = newSent, newRecv
				continue
			}

			sent, recv = newSent, newRecv
		case <-shutdownTimer.C:
			zap.S().Error("connection lost")
			gs.Shutdown()
			gs.Wait()
			return
		}
	}
}

type client interface {
	getProducerStats() (messages uint64, lossInvalidTopic, lossInvalidMessage, skipped uint64)
	getConsumerStats() (messages uint64, lossInvalidTopic, lossInvalidMessage, skipped uint64)
	startProducing(messageChan chan kafka.Message, commitChan chan *kafka.Message, split int)
	startConsuming(messageChan chan kafka.Message, commitChan chan *kafka.Message)
	shutdown() error
}

func newClient(broker, topic, serialNumber string) (client, error) {
	if strings.HasSuffix(broker, "1883") || strings.HasSuffix(broker, "8883") {
		return newMqttClient(broker, topic, serialNumber)
	}
	return newKafkaClient(broker, topic, serialNumber)
}
