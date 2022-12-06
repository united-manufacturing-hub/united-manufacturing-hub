package main

import (
	"encoding/binary"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strings"
)
import "github.com/hashicorp/golang-lru"

func CheckIfNewMessageOrStore(message []byte, topic string) (isNewMessage bool) {
	// Convert topic to lowercase and kafka format
	topic = strings.ToLower(topic)
	topic = strings.ReplaceAll(topic, "/", ".")

	if strings.HasPrefix(topic, "ia.raw") {
		return checkRawMessage(message, topic)
	} else {
		return checkMessage(message, topic)
	}
}

func computeHashB(message []byte, topic string) string {
	hashStringMessage := xxh3.Hash(message)
	hashStringTopic := xxh3.Hash([]byte(topic))

	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[:8], hashStringMessage)
	binary.LittleEndian.PutUint64(b[8:], hashStringTopic)
	return string(b)
}

var RawMessageLRU *lru.ARCCache
var MessageLRU *lru.ARCCache

func checkMessage(message []byte, topic string) bool {
	// Parse message to get the timestamp
	var messageMap map[string]interface{}
	err := jsoniter.Unmarshal(message, &messageMap)
	if err != nil {
		// If we can't parse the message, we assume it's new
		zap.S().Errorf("Error parsing message: %s [%v] (%s)", err, messageMap, topic)
		return false
	}
	timestamp, ok := messageMap["timestamp_ms"]
	if !ok {
		// If we can't find the timestamp, we assume it's new
		zap.S().Errorf("Error parsing message: no timestamp [%v] (%s)", messageMap, topic)
		return false
	}

	// Compute hash
	hash := computeHashB(message, topic)

	// Check if we already have this message

	var cacheKey string
	switch t := timestamp.(type) {
	case float64:
		cacheKey = fmt.Sprintf("%f", t)
	case int:
		cacheKey = fmt.Sprintf("%d", t)
	case int64:
		cacheKey = fmt.Sprintf("%d", t)
	case string:
		cacheKey = t
	default:
		zap.S().Errorf("Error parsing message: unknown timestamp type [%v] (%s)", messageMap, topic)
	}

	cacheKey += hash
	_, ok = MessageLRU.Get(cacheKey)
	if ok {
		// Already seen
		return false
	} else {
		// New message
		MessageLRU.Add(cacheKey, true)
		return true
	}
}

func checkRawMessage(message []byte, topic string) bool {
	hash := computeHashB(message, topic)
	_, ok := RawMessageLRU.Get(hash)
	if ok {
		// Already seen
		return false
	} else {
		// New message
		RawMessageLRU.Add(hash, true)
		return true
	}
}
