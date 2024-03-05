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
func (c *MockConnection) GetMarkedMessageCount() uint64 {
	// This is totally unsafe, but should be ok for testing
	// If at a later date, you have race conditions here, use a mutex around the marked slice
	return uint64(len(c.Marked))
}

func GetMockKafkaClient(t *testing.T, msgchan chan *shared.KafkaMessage) IConnection {
	// Passing t here to ensure it is not used in production code
	t.Logf("Using mock client")
	return &MockConnection{
		MessagesToSend: msgchan,
		Marked:         make([]*shared.KafkaMessage, 0),
	}
}
