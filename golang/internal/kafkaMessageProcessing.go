//go:build kafkaALO
// +build kafkaALO

package internal

import (
	"bytes"
	"encoding/gob"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/coocood/freecache"
	"go.uber.org/zap"
	"regexp"
)

// This regexp is used to extract the topic name from the message key
var rp = regexp.MustCompile(`ia\.([\w]*)\.([\w]*)\.([\w]*)\.([\w]*)`)

// ParsedMessage is a struct that contains the parsed message key and value as AssetId, Location, CustomerId, PayloadType & Payload
type ParsedMessage struct {
	AssetId     string
	Location    string
	CustomerId  string
	PayloadType string
	Payload     []byte
}

// ParseMessage parses a kafka message and returns a ParsedMessage struct or false if the message is not a valid message
func ParseMessage(msg *kafka.Message) (bool, ParsedMessage) {

	valid, found, message := GetCacheParsedMessage(msg)
	if !valid {
		return false, ParsedMessage{}
	}
	if found {
		return true, message
	}

	valid, m := PutCacheKafkaMessageAsParsedMessage(msg)
	if !valid {
		return false, ParsedMessage{}
	}

	return true, m

}

// Messagecache is a cache of messages, this prevents double parsing of messages
var Messagecache *freecache.Cache

func InitMessageCache(messageCacheSizeBytes int) {
	Messagecache = freecache.NewCache(messageCacheSizeBytes)
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
		err = Messagecache.Set(cacheKey, buffer.Bytes(), 0)
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
	get, err := Messagecache.Get(cacheKey)
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
