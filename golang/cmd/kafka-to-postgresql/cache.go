package main

import (
	"encoding/binary"
	"github.com/coocood/freecache"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"strings"
)

// dbcache is a cache of database entries (for example database asset id's)
var dbcache *freecache.Cache

// InitCache initializes the cache with the given sizes in byte
func InitCache(DBcacheSizeBytes int) {
	zap.S().Infof("Initializing cache with DB cache size %d bytes", DBcacheSizeBytes)
	dbcache = freecache.NewCache(DBcacheSizeBytes)
}

// GetUint32DB looks up the DB cache for the key and returns the value if found as a uint32
func GetUint32DB(key []byte) (uint32, bool) {
	if dbcache == nil {
		return 0, false
	}
	value, err := dbcache.Get(key)
	if err != nil {
		return 0, false
	}
	return BytesToUint32(value), true
}

// PutUint32DB puts the key and value into the DB cache
func PutUint32DB(key []byte, value uint32) {
	if dbcache == nil {
		return
	}
	_ = dbcache.Set(key, Uint32ToBytes(value), 0)
}

// Uint32ToBytes converts a uint32 to a byte array
func Uint32ToBytes(a uint32) (b []byte) {
	b = make([]byte, 4)
	binary.LittleEndian.PutUint32(b, a)
	return
}

// BytesToUint32 converts a byte array to a uint32
func BytesToUint32(b []byte) (a uint32) {
	return binary.LittleEndian.Uint32(b)
}

// GetCacheAssetTableId looks up the db cache for the customerId, locationId, assetId and returns the asset table id if found
func GetCacheAssetTableId(customerID string, locationID string, assetID string) (uint32, bool) {
	return GetUint32DB(internal.AsXXHash([]byte(customerID), []byte(locationID), []byte(assetID)))
}

// PutCacheAssetTableId puts the assetTableId into the db cache, using the customerId, locationId, assetId as the key
func PutCacheAssetTableId(customerID string, locationID string, assetID string, assetTableId uint32) {
	PutUint32DB(internal.AsXXHash([]byte(customerID), []byte(locationID), []byte(assetID)), assetTableId)
}

// PutCacheProductTableId puts the productTableId into the db cache, using the customerId, locationId, productId as the key
func PutCacheProductTableId(customerID string, AssetTableId uint32, productTableId uint32) {
	PutUint32DB(internal.AsXXHash([]byte(customerID), Uint32ToBytes(AssetTableId)), productTableId)
}

// GetCacheProductTableId looks up the db cache for the customerId, locationId, productId and returns the product table id if found
func GetCacheProductTableId(customerID string, AssetTableId uint32) (uint32, bool) {
	return GetUint32DB(internal.AsXXHash([]byte(customerID), Uint32ToBytes(AssetTableId)))
}

// PutCacheKafkaMessageAsParsedMessage tries to parse the kafka message and put it into the message cache, returning the parsed message if successful
func PutCacheKafkaMessageAsParsedMessage(msg *kafka.Message) (valid bool, message ParsedMessage) {
	valid = false
	if msg == nil || msg.TopicPartition.Topic == nil {
		return
	}
	res := rp.FindStringSubmatch(*msg.TopicPartition.Topic)
	if res == nil {
		zap.S().Errorf(" Invalid topic: %s", *msg.TopicPartition.Topic)
		return false, ParsedMessage{}
	}

	customerID := res[1]
	location := res[2]
	assetID := res[3]
	payloadType := strings.ToLower(res[4])
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

// GetCacheParsedMessage looks up the message cache for the key and returns the parsed message if found
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
