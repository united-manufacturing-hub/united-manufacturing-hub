package kafka

import (
	"errors"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/consumer/redpanda"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Connection struct {
	consumer *redpanda.Consumer
}

var conn *Connection
var once sync.Once

func GetOrInit() *Connection {
	once.Do(func() {
		zap.S().Debugf("kafka.GetOrInit().once")
		KafkaBrokers, err := env.GetAsString("KAFKA_BROKERS", true, "http://united-manufacturing-hub-kafka.united-manufacturing-hub.svc.cluster.local:9092")
		if err != nil {
			zap.S().Fatalf("Failed to get KAFKA_BROKERS from env")
		}
		KafkaHTTPBrokers, err := env.GetAsString("KAFKA_HTTP_BROKERS", true, "http://united-manufacturing-hub-kafka.united-manufacturing-hub.svc.cluster.local:8082")
		if err != nil {
			zap.S().Fatalf("Failed to get KAFKA_HTTP_BROKERS from env")
		}

		brokers := strings.Split(KafkaBrokers, ",")
		httpBrokers := strings.Split(KafkaHTTPBrokers, ",")
		instanceID := rand.Int63() //nolint:gosec

		consumer, err := redpanda.NewConsumer(brokers, httpBrokers, []string{"^umh\\.v1.+$"}, "kafka-to-postgresql-v2", strconv.FormatInt(instanceID, 10), false)
		if err != nil {
			zap.S().Fatalf("Failed to create kafka client: %s", err)
		}
		zap.S().Debugf("kafka.GetOrInit().once.consumer.Start()")
		err = consumer.Start()
		zap.S().Debugf("post kafka.GetOrInit().once.consumer.Start()")
		if err != nil {
			zap.S().Fatalf("Failed to start consumer: %s", err)
		}
		conn = &Connection{
			consumer: consumer,
		}
	})
	return conn
}

func (c *Connection) GetMessages() chan *shared.KafkaMessage {
	return c.consumer.GetMessages()
}

func (c *Connection) MarkMessage(message *shared.KafkaMessage) {
	c.consumer.MarkMessage(message)
}

var lastMarked atomic.Uint64
var lastChangeUTCSeconds atomic.Int64

func GetLivenessCheck() healthcheck.Check {
	return func() error {
		marked, _ := GetOrInit().consumer.GetStats()
		oldValue := lastMarked.Swap(marked)
		nowUTCSeconds := time.Now().UTC().Unix()
		if oldValue < marked {
			lastChangeUTCSeconds.Store(nowUTCSeconds)
			return nil
		} else if oldValue > marked {
			return errors.New("amount of marked messages went down")
		} else {
			// Check if last change is more then 5 minutes ago
			lastChange := lastChangeUTCSeconds.Load()
			elapsedSeconds := nowUTCSeconds - lastChange
			if elapsedSeconds > 60*5 {
				return errors.New("no new kafka message in the last 5 minutes")
			} else {
				return nil
			}
		}
	}
}

func GetReadinessCheck() healthcheck.Check {
	return func() error {
		if GetOrInit().consumer.IsRunning() {
			return nil
		} else {
			return errors.New("kafka consumer is not running")
		}
	}
}
