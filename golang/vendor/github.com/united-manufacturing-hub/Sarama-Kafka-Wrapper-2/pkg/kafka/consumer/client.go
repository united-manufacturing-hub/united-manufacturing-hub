package consumer

import (
	"context"
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"sync/atomic"
)

// Consumer wraps sarama's ConsumerGroup.
type Consumer struct {
	consumerGroup         *sarama.ConsumerGroup
	incomingMessages      chan *shared.KafkaMessage
	consumerContextCancel context.CancelFunc
	messagesToMark        chan *shared.KafkaMessage
	topic                 string
	brokers               []string
	markedMessages        atomic.Uint64
	consumedMessages      atomic.Uint64
	running               atomic.Bool
}

// NewConsumer initializes a Consumer.
func NewConsumer(brokers []string, topic, groupName string) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	cg, err := sarama.NewConsumerGroup(brokers, groupName, config)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		brokers:          brokers,
		topic:            topic,
		consumerGroup:    &cg,
		incomingMessages: make(chan *shared.KafkaMessage, 100_000),
		messagesToMark:   make(chan *shared.KafkaMessage, 100_000),
	}, nil
}

// Start runs the Consumer.
func (c *Consumer) Start(ctx context.Context) error {
	if c.running.Swap(true) {
		return nil
	}
	internalCtx, cancel := context.WithCancel(ctx)
	c.consumerContextCancel = cancel
	go c.consume(internalCtx)
	return nil
}

func (c *Consumer) consume(ctx context.Context) {
	for c.running.Load() {
		handler := &GroupHandler{
			incomingMessages: c.incomingMessages,
			messagesToMark:   c.messagesToMark,
			running:          &c.running,
			markedMessages:   &c.markedMessages,
			consumedMessages: &c.consumedMessages,
		}

		if err := (*c.consumerGroup).Consume(ctx, []string{c.topic}, handler); err != nil {
			c.running.Store(false)
			zap.S().Error(err)
		}
	}
}

// Close terminates the Consumer.
func (c *Consumer) Close() error {
	if !c.running.Swap(false) {
		return nil
	}
	c.consumerContextCancel()
	return (*c.consumerGroup).Close()
}

// IsRunning returns the run state.
func (c *Consumer) IsRunning() bool {
	return c.running.Load()
}

// GetMessage receives a single message.
func (c *Consumer) GetMessage() *shared.KafkaMessage {
	select {
	case msg := <-c.incomingMessages:
		return msg
	default:
		return nil
	}
}

// GetMessages returns the message channel.
func (c *Consumer) GetMessages() chan *shared.KafkaMessage {
	return c.incomingMessages
}

// MarkMessage marks a message for commit.
func (c *Consumer) MarkMessage(msg *shared.KafkaMessage) {
	c.messagesToMark <- msg
}

// MarkMessages marks multiple messages for commit.
func (c *Consumer) MarkMessages(msgs []*shared.KafkaMessage) {
	for _, msg := range msgs {
		c.messagesToMark <- msg
	}
}

// GetStats returns marked and consumed message counts.
func (c *Consumer) GetStats() (uint64, uint64) {
	return c.markedMessages.Load(), c.consumedMessages.Load()
}
