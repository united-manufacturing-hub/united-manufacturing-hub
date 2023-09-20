package kafka

import (
	"github.com/IBM/sarama"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Message struct {
	Topic     string
	Value     []byte
	Header    map[string][]byte
	Key       []byte
	Offset    int64
	Partition int32
}

func ToKafkaMessage(topic string, message interface{}) (Message, error) {
	bytes, err := json.Marshal(message)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Topic: topic,
		Value: bytes,
	}, nil
}

func MessageToProducerMessage(message *Message) *sarama.ProducerMessage {
	records := make([]sarama.RecordHeader, 0, len(message.Header))
	for s, bytes := range message.Header {
		records = append(records, sarama.RecordHeader{Key: []byte(s), Value: bytes})
	}

	prodMsg := sarama.ProducerMessage{
		Topic:    message.Topic,
		Key:      sarama.ByteEncoder(message.Key),
		Headers:  records,
		Value:    sarama.ByteEncoder(message.Value),
		Metadata: sarama.MetadataRequest{AllowAutoTopicCreation: true},
	}
	return &prodMsg
}

func ProducerMessageToMessage(prodMsg *sarama.ProducerMessage) (*Message, error) {
	encodedValue, err := prodMsg.Value.Encode()
	if err != nil {
		return nil, err
	}

	encodedKey, err := prodMsg.Key.Encode()
	if err != nil {
		return nil, err
	}

	headers := make(map[string][]byte)

	for _, v := range prodMsg.Headers {
		headers[string(v.Key)] = v.Value
	}

	convertedMessage := Message{
		Topic:  prodMsg.Topic,
		Value:  encodedValue,
		Header: headers,
		Key:    encodedKey,
	}
	return &convertedMessage, nil
}

func MessageToConsumerMessage(message *Message) *sarama.ConsumerMessage {
	records := make([]*sarama.RecordHeader, 0, len(message.Header))
	for s, bytes := range message.Header {
		records = append(records, &sarama.RecordHeader{Key: []byte(s), Value: bytes})
	}

	convertedMessage := sarama.ConsumerMessage{
		Headers:   records,
		Topic:     message.Topic,
		Key:       message.Key,
		Value:     message.Value,
		Partition: message.Partition,
		Offset:    message.Offset,
	}

	return &convertedMessage
}

func ProducerMessageToConsumerMessage(message *sarama.ProducerMessage) (*sarama.ConsumerMessage, error) {

	encodedValue, err := message.Value.Encode()
	if err != nil {
		return nil, err
	}

	encodedKey, err := message.Key.Encode()
	if err != nil {
		return nil, err
	}

	records := make([]*sarama.RecordHeader, 0, len(message.Headers))
	for _, header := range message.Headers {
		records = append(records, &sarama.RecordHeader{Key: header.Key, Value: header.Value})
	}

	convertedMessage := sarama.ConsumerMessage{
		Topic:     message.Topic,
		Key:       encodedKey,
		Value:     encodedValue,
		Headers:   records,
		Partition: message.Partition,
		Offset:    message.Offset,
		Timestamp: message.Timestamp,
	}

	return &convertedMessage, nil
}
