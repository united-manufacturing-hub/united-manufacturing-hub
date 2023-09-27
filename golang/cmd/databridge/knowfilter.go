package main

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"golang.org/x/crypto/sha3"
	"sync/atomic"
)

var arc *lru.ARCCache
var hits atomic.Uint64
var misses atomic.Uint64

func QueryOrInsert(msg *shared.KafkaMessage) (known bool) {
	if arc == nil {
		arc, _ = lru.NewARC(1000000)
	}

	// Generate hash
	hasher := sha3.New512()
	_, _ = hasher.Write([]byte(msg.Topic))
	_, _ = hasher.Write(msg.Value)
	hash := hasher.Sum(nil)
	// hash to string
	hashStr := string(hash)

	if _, ok := arc.Get(hashStr); ok {
		hits.Add(1)
		return true
	}
	misses.Add(1)
	arc.Add(hashStr, true)

	return false
}

func GetLRUStats() (h uint64, m uint64, s int) {
	return hits.Load(), misses.Load(), arc.Len()
}
