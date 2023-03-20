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

	zap.S().Debugf("Checking if new message: %s [%s]", topic, message)

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
	var cacheKey string
	if ok {
		// Compute hash
		hash := computeHashB(message, topic)

		// Check if we already have this message
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
	} else {
		// If we can't find the timestamp, use the hash of the whole message
		cacheKey = computeHashB(message, topic)
		return true
	}

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
