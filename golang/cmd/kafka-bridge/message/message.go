package message

import (
	lru "github.com/hashicorp/golang-lru"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
	"strings"
)

var arcRaw *lru.ARCCache
var arcNonRaw *lru.ARCCache

var arcSizeRaw = 1_000_000
var arcSizeNonRaw = 1_000_000

func Init() {
	if arcRaw != nil {
		return
	}
	arcSizeRaw, err := env.GetAsInt("RAW_MESSSAGE_LRU_SIZE", false, 1_000_000)
	if err != nil {
		zap.S().Warn(err)
	}
	arcSizeNonRaw, err := env.GetAsInt("MESSAGE_LRU_SIZE", false, 1_000_000)
	if err != nil {
		zap.S().Warn(err)
	}

	arcRaw, _ = lru.NewARC(arcSizeRaw)
	arcNonRaw, _ = lru.NewARC(arcSizeNonRaw)
}

func GetCacheSize() (int, int, int, int) {
	return arcRaw.Len(), arcSizeRaw, arcNonRaw.Len(), arcSizeNonRaw
}

func IsValidKafkaMessage(message kafka.Message) bool {
	if !isValid(message.Topic, message.Value) {
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

func isValid(topic string, payload []byte) bool {
	if strings.HasPrefix(topic, ".") {
		zap.S().Warnf("Topic starts with a dot: %s", topic)
		return false
	}
	if strings.HasSuffix(topic, ".") {
		zap.S().Warnf("Topic ends with a dot: %s", topic)
		return false
	}

	isRaw := strings.HasPrefix(topic, "ia.raw")

	if !isRaw {
		// Check if payload is a valid json
		if !jsoniter.Valid(payload) {
			zap.S().Warnf("Not a valid json: %s: %s", topic, string(payload))
			return false
		}
	}

	if isRaw {
		// Check if message is known
		hasher := xxh3.New()
		_, _ = hasher.Write([]byte(topic))
		_, _ = hasher.Write(payload)
		hash := hasher.Sum64()

		// Uses Get to re-validate the entry
		if _, ok := arcRaw.Get(hash); ok {
			return false
		}
		arcRaw.Add(hash, true)
	} else {
		// Check if message is known
		hasher := sha3.New512()
		_, _ = hasher.Write([]byte(topic))
		_, _ = hasher.Write(payload)
		hash := hasher.Sum(nil)
		// hash to string
		hashStr := string(hash)

		// Uses Get to re-validate the entry
		if _, ok := arcNonRaw.Get(hashStr); ok {
			return false
		}
		arcNonRaw.Add(hashStr, true)
	}

	return true
}
