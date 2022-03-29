package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"github.com/cespare/xxhash/v2"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/coocood/freecache"
	"go.uber.org/zap"
)

func AsXXHash(inputs ...[]byte) []byte {
	var h = xxhash.New()
	for _, input := range inputs {
		_, _ = h.Write(input)
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, h.Sum64())
	return b
}

var dbcache *freecache.Cache
var messagecache *freecache.Cache

func InitCache(DBcacheSizeBytes int, messageCacheSizeBytes int) {
	dbcache = freecache.NewCache(DBcacheSizeBytes)
	messagecache = freecache.NewCache(messageCacheSizeBytes)
}

func GetUint32(key []byte) (uint32, bool) {
	if dbcache == nil {
		return 0, false
	}
	value, err := dbcache.Get(key)
	if err != nil {
		return 0, false
	}
	return binary.LittleEndian.Uint32(value), true
}

func PutUint32(key []byte, value uint32) {
	if dbcache == nil {
		return
	}
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, value)
	_ = dbcache.Set(key, b, 0)
}

func GetCacheAssetTableId(customerID string, locationID string, assetID string) (uint32, bool) {
	return GetUint32(AsXXHash([]byte(customerID), []byte(locationID), []byte(assetID)))
}

func PutCacheAssetTableId(customerID string, locationID string, assetID string, assetTableId uint32) {
	PutUint32(AsXXHash([]byte(customerID), []byte(locationID), []byte(assetID)), assetTableId)
}

func PutCacheParsedMessage(msg *kafka.Message) (valid bool, message ParsedMessage) {
	valid = false
	if msg == nil || msg.TopicPartition.Topic == nil {
		return
	}
	res := rp.FindStringSubmatch(*msg.TopicPartition.Topic)
	if res == nil {
		zap.S().Errorf(" Invalid topic: %s", *msg.TopicPartition.Topic)
		return false, ParsedMessage{}
	}

	assetID := res[1]
	location := res[2]
	customerID := res[3]
	payloadType := res[4]
	payload := msg.Value
	pm := ParsedMessage{
		AssetId:     assetID,
		Location:    location,
		CustomerId:  customerID,
		PayloadType: payloadType,
		Payload:     payload,
	}

	var cacheKey = AsXXHash(msg.Key, msg.Value, []byte((*msg.TopicPartition.Topic)))

	var buffer bytes.Buffer
	err := gob.NewEncoder(&buffer).Encode(pm)
	if err != nil {
		zap.S().Errorf("Failed to encode message: %s", err)
	} else {
		err = messagecache.Set(cacheKey, buffer.Bytes(), 0)
		if err != nil {
			zap.S().Debugf("Error putting message in cache: %s", err)
		}
	}

	return true, pm
}

func GetCacheParsedMessage(msg *kafka.Message) (valid bool, found bool, message ParsedMessage) {
	if msg == nil || msg.TopicPartition.Topic == nil {
		return false, false, ParsedMessage{}
	}

	var cacheKey = AsXXHash(msg.Key, msg.Value, []byte((*msg.TopicPartition.Topic)))
	get, err := messagecache.Get(cacheKey)
	if err != nil {
		return true, false, ParsedMessage{}
	}

	var pm ParsedMessage
	reader := bytes.NewReader(get)
	err = gob.NewDecoder(reader).Decode(&pm)
	if err != nil {
		return false, true, ParsedMessage{}
	}

	return true, true, pm
}
