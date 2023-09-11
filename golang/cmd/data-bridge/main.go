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

	zap.S().Debug("Checking environment variables")
	serialNumber, err := env.GetAsString("SERIAL_NUMBER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	microserviceName, err := env.GetAsString("MICROSERVICE_NAME", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	mode, err := env.GetAsInt("MODE", true, -1)
	if err != nil {
		zap.S().Fatal(err)
	}
	if mode != 0 && mode != 1 {
		zap.S().Fatal("invalid MODE")
	}
	bridgeMode, err := env.GetAsInt("BRIDGE_MODE", true, -1)
	if err != nil {
		zap.S().Fatal(err)
	}
	if bridgeMode != 0 && bridgeMode != 1 {
		zap.S().Fatal("invalid BRIDGE_MODE")
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
		zap.S().Fatal("SPLIT must be at least 3")
	}

	partitons, err := env.GetAsInt("PARTITIONS", false, 6)
	if err != nil {
		zap.S().Error(err)
	}
	if partitons < 1 {
		zap.S().Fatal("PARTITIONS must be at least 1")
	}
	replicationFactor, err := env.GetAsInt("REPLICATION_FACTOR", false, 1)
	if err != nil {
		zap.S().Error(err)
	}
	if replicationFactor%2 == 0 {
		zap.S().Fatal("REPLICATION_FACTOR must be odd")
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
	if mode == 0 {
		// kafka to kafka
		clientA, err = newKafkaClient(brokerA, topic, partitons, replicationFactor)
		if err != nil {
			zap.S().Errorf("failed to create kafka client: %s", err)
		}
		clientB, err = newKafkaClient(brokerB, topic, partitons, replicationFactor)
		if err != nil {
			zap.S().Errorf("failed to create kafka client: %s", err)
		}
	} else {
		// mqtt to kafka
		mqttUseTls, err := env.GetAsBool("MQTT_ENABLE_TLS", false, false)
		if err != nil {
			zap.S().Error(err)
		}
		mqttPsw, err := env.GetAsString("MQTT_PASSWORD", false, "")
		if err != nil {
			zap.S().Error(err)
		}
		if strings.HasSuffix(brokerA, "1883") || strings.HasSuffix(brokerA, "8883") {
			clientA, err = newMqttClient(brokerA, topic, mqttUseTls, mqttPsw)
			if err != nil {
				zap.S().Errorf("failed to create mqtt client: %s", err)
			}
			clientB, err = newKafkaClient(brokerB, topic, partitons, replicationFactor)
			if err != nil {
				zap.S().Errorf("failed to create kafka client: %s", err)
			}
		} else {
			clientA, err = newKafkaClient(brokerA, topic, partitons, replicationFactor)
			if err != nil {
				zap.S().Errorf("failed to create kafka client: %s", err)
			}
			clientB, err = newMqttClient(brokerB, topic, mqttUseTls, mqttPsw)
			if err != nil {
				zap.S().Errorf("failed to create mqtt client: %s", err)
			}
		}
	}

	gs := internal.NewGracefulShutdown(func() error {
		clientA.shutdown()
		clientB.shutdown()
		return nil
	})

	var msgChan = make(chan kafka.Message, 100)

	if bridgeMode == 0 {
		// a to b
		clientA.startConsuming(msgChan)
		clientB.startProducing(msgChan, split)
		go reportStats(msgChan, clientA, clientB, gs)
	} else {
		// b to a
		clientA.startProducing(msgChan, split)
		clientB.startConsuming(msgChan)
		go reportStats(msgChan, clientB, clientA, gs)
	}

}

func reportStats(msgChan chan kafka.Message, consumerClient, producerClient client, gs internal.GracefulShutdownHandler) {
	var sent, recv uint64
	sent = producerClient.getProducerStats()
	recv = consumerClient.getConsumerStats()

	ticker := time.NewTicker(10 * time.Second)
	shutdownTimer := time.NewTimer(3 * time.Minute)
	for {
		select {
		case <-ticker.C:
			var newSent, newRecv uint64
			newSent = producerClient.getProducerStats()
			newRecv = consumerClient.getConsumerStats()

			sentPerSecond := (newSent - sent) / 10
			recvPerSecond := (newRecv - recv) / 10

			zap.S().Infof("Recieved: %d (%d/) | Sent: %d (%d/s) | Lag: %d", newSent, sentPerSecond, newRecv, recvPerSecond, len(msgChan))

			if newSent != sent && newRecv != recv {
				shutdownTimer.Reset(3 * time.Minute)
				return
			}

			sent, recv = newSent, newRecv
		case <-shutdownTimer.C:
			zap.S().Error("connection lost")
			gs.Shutdown()
			return
		}
	}
}

type client interface {
	getProducerStats() (messages uint64)
	getConsumerStats() (messages uint64)
	startProducing(messageChan chan kafka.Message, split int)
	startConsuming(messageChan chan kafka.Message)
	shutdown() error
}
