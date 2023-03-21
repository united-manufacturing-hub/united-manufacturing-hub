package message

import (
	lru "github.com/hashicorp/golang-lru"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strings"
)

var arcCache *lru.ARCCache

func Init() {
	if arcCache != nil {
		return
	}
	arcCache, _ = lru.NewARC(10_000_000)
}

func IsValidMQTTMessage(topic string, payload []byte) bool {
	return IsValid(topic, payload)
}

func IsValidKafkaMessage(message kafka.Message) bool {
	if !IsValid(message.Topic, message.Value) {
		return false
	}
	// Check if in x-origin
	if internal.IsSameOrigin(&message) {
		return false
	}

	// Check if in x-trace
	if internal.IsInTrace(&message) {
		return false
	}

	return true
}

func IsValid(topic string, payload []byte) bool {
	kafkaTopicName := strings.ReplaceAll(topic, "/", ".")
	if strings.HasPrefix(kafkaTopicName, ".") {
		zap.S().Warnf("Topic starts with a dot: %s", topic)
		return false
	}
	if strings.HasSuffix(kafkaTopicName, ".") {
		zap.S().Warnf("Topic ends with a dot: %s", topic)
		return false
	}

	if !strings.HasPrefix(kafkaTopicName, "ia.raw") {
		// Check if payload is a valid json
		if !jsoniter.Valid(payload) {
			zap.S().Warnf("Not a valid json: %s: %s", topic, string(payload))
			return false
		}
	}

	// Check if message is known
	hasher := xxh3.New()
	_, _ = hasher.Write([]byte(topic))
	_, _ = hasher.Write(payload)
	hash := hasher.Sum64()

	if _, ok := arcCache.Get(hash); ok {
		return false
	}
	arcCache.Add(hash, true)

	return true
}
