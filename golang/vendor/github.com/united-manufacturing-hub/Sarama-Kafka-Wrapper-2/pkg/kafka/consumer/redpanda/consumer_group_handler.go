package redpanda

import (
	"errors"
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"sync/atomic"
)

// ConsumerGroupHandler represents a Sarama consumer group consumer
type ConsumerGroupHandler struct {
	ready              *atomic.Bool
	incomingMessages   chan *shared.KafkaMessage
	messagesToMarkChan chan *shared.KafkaMessage
	read               *atomic.Uint64
	marked             *atomic.Uint64
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (c *ConsumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	if c.ready == nil {
		return errors.New("ConsumerGroupHandler: ready channel is nil")
	}
	if c.incomingMessages == nil {
		return errors.New("ConsumerGroupHandler: incomingMessages channel is nil")
	}
	if c.messagesToMarkChan == nil {
		return errors.New("ConsumerGroupHandler: messagesToMarkChan channel is nil")
	}
	if c.read == nil {
		return errors.New("ConsumerGroupHandler: read counter is nil")
	}
	if c.marked == nil {
		return errors.New("ConsumerGroupHandler: marked counter is nil")
	}

	c.ready.Store(true)
	zap.S().Debugf("ConsumerGroupHandler set up for: %+v", session.Claims())
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (c *ConsumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	zap.S().Debugf("ConsumerGroupHandler cleaned up")
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
// Once the Messages() channel is closed, the Handler must finish its processing
// loop and exit.
func (c *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	zap.S().Debugf("ConsumerGroupHandler: starting to consume claim: %+v", claim)
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				zap.S().Infof("ConsumerGroupHandler: Message channel closed")
				return nil
			}
			c.incomingMessages <- &shared.KafkaMessage{
				Topic:     message.Topic,
				Partition: message.Partition,
				Offset:    message.Offset,
				Key:       message.Key,
				Value:     message.Value,
			}
			c.read.Add(1)
		case msg := <-c.messagesToMarkChan:
			session.MarkOffset(msg.Topic, msg.Partition, msg.Offset+1, "")
			c.marked.Add(1)
		// Should return when `session.Context()` is done.
		// If not, will raise `ErrRebalanceInProgress` or `read tcp <ip>:<port>: i/o timeout` when kafka rebalances. see:
		// https://github.com/IBM/sarama/issues/1192
		case _, ok := <-session.Context().Done():
			if !ok {
				zap.S().Infof("ConsumerGroupHandler: Session context channel closed")
				return nil
			}
			zap.S().Infof("ConsumerGroupHandler: Session context closed")
			return nil
		}
	}
}
