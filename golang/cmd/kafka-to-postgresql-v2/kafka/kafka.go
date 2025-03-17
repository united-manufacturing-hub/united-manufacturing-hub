package kafka

import (
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/IBM/sarama"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/consumer/redpanda"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
)

type IConnection interface {
	GetMessages() <-chan *shared.KafkaMessage
	MarkMessage(message *shared.KafkaMessage)
	GetMarkedMessageCount() uint64
}

type Connection struct {
	consumer *redpanda.Consumer
	marked   atomic.Uint64
}

func (c *Connection) GetMarkedMessageCount() uint64 {
	return c.marked.Load()
}

var conn *Connection

func GetKafkaClient() IConnection {
	return conn
}

func InitKafkaClient() {
	zap.S().Debugf("kafka.GetKafkaClient()")
	KafkaBrokers, err := env.GetAsString("KAFKA_BROKERS", true, "http://united-manufacturing-hub-kafka.united-manufacturing-hub.svc.cluster.local:9092")
	if err != nil {
		zap.S().Fatalf("Failed to get KAFKA_BROKERS from env")
	}

	brokers := strings.Split(KafkaBrokers, ",")
	instanceID := rand.Int63() //nolint:gosec

	consumer, err := redpanda.NewConsumer(brokers, []string{"^umh\\.v1.+$"}, "kafka-to-postgresql-v2", strconv.FormatInt(instanceID, 10), sarama.OffsetOldest)
	if err != nil {
		zap.S().Fatalf("Failed to create kafka client: %s", err)
	}
	conn = &Connection{
		consumer: consumer,
	}
}

func (c *Connection) GetMessages() <-chan *shared.KafkaMessage {
	return c.consumer.GetMessages()
}

func (c *Connection) MarkMessage(message *shared.KafkaMessage) {
	c.marked.Add(1)
	c.consumer.MarkMessage(message)
}

func GetLivenessCheck() healthcheck.Check {
	return func() error {
		// For now, the kafka client will always be alive (or it will panic [connection loss, etc.])
		return nil
	}
}

func GetReadinessCheck() healthcheck.Check {
	return func() error {
		client := GetKafkaClient()
		// Attempt to cast to Connection
		c, ok := client.(*Connection)
		if !ok {
			return errors.New("kafka client is not running or mocked")
		}
		if c.consumer.IsReady() {
			return nil
		} else {
			return errors.New("kafka consumer is not running")
		}
	}
}
