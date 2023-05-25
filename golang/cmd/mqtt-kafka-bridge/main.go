package main

import (
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/mqtt-kafka-bridge/kafka_processor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/mqtt-kafka-bridge/message"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/mqtt-kafka-bridge/mqtt_processor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

func main() {

	var err error
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err = logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	var mqttToKafkaChan = make(chan kafka.Message, 100)
	var kafkaToMqttChan = make(chan kafka.Message, 100)
	var shutdownChan = make(chan bool, 1)

	message.Init()
	kafka_processor.Init(kafkaToMqttChan, shutdownChan)
	mqtt_processor.Init(mqttToKafkaChan, shutdownChan)

	kafka_processor.Start(mqttToKafkaChan)
	mqtt_processor.Start(kafkaToMqttChan)

	go checkDisconnect(shutdownChan)
	reportStats(shutdownChan, mqttToKafkaChan, kafkaToMqttChan)

	zap.S().Info("Shutting down")
	kafka_processor.Shutdown()
	mqtt_processor.Shutdown()
}

func checkDisconnect(shutdownChan chan bool) {
	var kafkaSent, kafkaRecv, mqttSent, mqttRecv uint64
	kafkaSent, kafkaRecv, _, _ = kafka_processor.GetStats()
	mqttSent, mqttRecv = mqtt_processor.GetStats()

	for {
		time.Sleep(3 * time.Minute)
		newKafkaSent, newKafkaRecv, _, _ := kafka_processor.GetStats()
		newMqttSent, newMqttRecv := mqtt_processor.GetStats()
		if newMqttSent == mqttSent && newMqttRecv == mqttRecv {
			zap.S().Error("MQTT connection lost")
			shutdownChan <- true
			return
		}
		if newKafkaSent == kafkaSent && newKafkaRecv == kafkaRecv {
			zap.S().Error("Kafka connection lost")
			shutdownChan <- true
			return
		}
		kafkaSent = newKafkaSent
		kafkaRecv = newKafkaRecv
		mqttSent = newMqttSent
		mqttRecv = newMqttRecv
	}
}

func reportStats(shutdownChan chan bool, mqttToKafkaChan chan kafka.Message, kafkaToMqttChan chan kafka.Message) {
	var kafkaSent, kafkaRecv, mqttSent, mqttRecv uint64
	kafkaSent, kafkaRecv, _, _ = kafka_processor.GetStats()
	mqttSent, mqttRecv = mqtt_processor.GetStats()
	for {
		select {
		case <-shutdownChan:
			return
		case <-time.After(10 * time.Second):
			// Calculate per second
			newKafkaSent, newKafkaRecv, _, _ := kafka_processor.GetStats()
			newMqttSent, newMqttRecv := mqtt_processor.GetStats()

			kafkaSentPerSecond := (newKafkaSent - kafkaSent) / 10
			kafkaRecvPerSecond := (newKafkaRecv - kafkaRecv) / 10
			mqttSentPerSecond := (newMqttSent - mqttSent) / 10
			mqttRecvPerSecond := (newMqttRecv - mqttRecv) / 10
			cacheUsedRaw, cacheMaxRaw, cacheUsed, cacheMax := message.GetCacheSize()
			cachePercentRaw := float64(cacheUsedRaw) / float64(cacheMaxRaw) * 100
			cachePercent := float64(cacheUsed) / float64(cacheMax) * 100
			zap.S().Infof(
				"Kafka sent: %d (%d/s), Kafka recv: %d (%d/s) | MQTT sent: %d (%d/s), MQTT recv: %d (%d/s) | Cached: %d/%d (%.2f%%), Cached raw: %d/%d (%.2f%%) | MqttToKafka chan: %d | KafkaToMqtt chan: %d",
				newKafkaSent, kafkaSentPerSecond, newKafkaRecv, kafkaRecvPerSecond, newMqttSent, mqttSentPerSecond, newMqttRecv, mqttRecvPerSecond, cacheUsed, cacheMax, cachePercent, cacheUsedRaw, cacheMaxRaw, cachePercentRaw, len(mqttToKafkaChan), len(kafkaToMqttChan))

			kafkaSent = newKafkaSent
			kafkaRecv = newKafkaRecv
			mqttSent = newMqttSent
			mqttRecv = newMqttRecv
		}
	}
}
