package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
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
