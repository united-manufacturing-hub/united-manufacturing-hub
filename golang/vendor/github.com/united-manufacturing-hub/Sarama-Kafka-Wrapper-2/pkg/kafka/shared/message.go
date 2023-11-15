package shared

import (
	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// KafkaMessage represents a message in the Kafka queue.
type KafkaMessage struct {
	Headers   map[string]string `json:"headers"`
	Topic     string            `json:"topic"`
	Key       []byte            `json:"key"`
	Value     []byte            `json:"value"`
	Offset    int64             `json:"offset"`
	Partition int32             `json:"partition"`
}

// FromConsumerMessage converts a sarama.ConsumerMessage to a KafkaMessage.
func FromConsumerMessage(message *sarama.ConsumerMessage) *KafkaMessage {
	if message == nil {
		return nil
	}
	m := &KafkaMessage{
		Headers:   make(map[string]string, len(message.Headers)),
		Key:       message.Key,
		Value:     message.Value,
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
	}
	for _, header := range message.Headers {
		m.Headers[string(header.Key)] = string(header.Value)
	}
	return m
}

// ToConsumerMessage converts a KafkaMessage to a sarama.ConsumerMessage.
func ToConsumerMessage(message *KafkaMessage) *sarama.ConsumerMessage {
	if message == nil {
		return nil
	}
	m := &sarama.ConsumerMessage{
		Key:       message.Key,
		Value:     message.Value,
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
	}
	m.Headers = make([]*sarama.RecordHeader, 0, len(message.Headers))
	for k, v := range message.Headers {
		m.Headers = append(m.Headers, &sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}
	return m
}

// ToProducerMessage converts a KafkaMessage to a sarama.ProducerMessage.
// It ignores the Partition and Offset fields and sets trace headers.
func ToProducerMessage(message *KafkaMessage) *sarama.ProducerMessage {
	if message == nil {
		return nil
	}
	if v, _ := GetSXOrigin(message); !v {
		AddSXOrigin(message)
	}
	if v, _ := GetSXTrace(message); !v {
		err := AddSXTrace(message)
		if err != nil {
			zap.S().Errorf("failed to add trace header: %s", err)
			return nil
		}
	}
	m := &sarama.ProducerMessage{
		Topic: message.Topic,
		Key:   sarama.ByteEncoder(message.Key),
		Value: sarama.ByteEncoder(message.Value),
	}
	m.Headers = make([]sarama.RecordHeader, 0, len(message.Headers))
	for k, v := range message.Headers {
		m.Headers = append(m.Headers, sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}
	return m
}
