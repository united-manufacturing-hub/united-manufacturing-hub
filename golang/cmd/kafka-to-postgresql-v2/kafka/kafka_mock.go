package kafka

import (
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"testing"
)

type MockConnection struct {
	MessagesToSend chan *shared.KafkaMessage
	Marked         []*shared.KafkaMessage
}

func (c *MockConnection) GetMessages() <-chan *shared.KafkaMessage {
	return c.MessagesToSend
}

func (c *MockConnection) MarkMessage(msg *shared.KafkaMessage) {
	c.Marked = append(c.Marked, msg)
}

func GetMockKafkaClient(t *testing.T) IConnection {
	// Passing t here to ensure it is not used in production code
	t.Logf("Using mock client")
	return &MockConnection{
		MessagesToSend: make(chan *shared.KafkaMessage),
		Marked:         make([]*shared.KafkaMessage, 0),
	}
}
