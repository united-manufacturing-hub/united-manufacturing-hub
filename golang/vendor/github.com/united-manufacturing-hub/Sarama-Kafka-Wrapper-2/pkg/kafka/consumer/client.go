package consumer

import (
	"context"
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"regexp"
	"sync/atomic"
	"time"
)

// Consumer wraps sarama's ConsumerGroup.
type Consumer struct {
	consumerGroup         *sarama.ConsumerGroup
	incomingMessages      chan *shared.KafkaMessage
	consumerContextCancel context.CancelFunc
	messagesToMark        chan *shared.KafkaMessage
	regexTopics           []regexp.Regexp
	brokers               []string
	markedMessages        atomic.Uint64
	consumedMessages      atomic.Uint64
	running               atomic.Bool
	actualTopics          []string
	internalCtx           context.Context
	rawClient             sarama.Client
}

// NewConsumer initializes a Consumer.
func NewConsumer(brokers, topic []string, groupName string) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	c, err := sarama.NewClient(brokers, config)
	if err != nil {
		return nil, err
	}
	zap.S().Infof("connected to brokers: %v", brokers)
	err = c.RefreshMetadata()
	if err != nil {
		return nil, err
	}
	zap.S().Info("Refreshed metadata")

	var cg sarama.ConsumerGroup
	cg, err = sarama.NewConsumerGroupFromClient(groupName, c)
	if err != nil {
		return nil, err
	}

	var rgxTopics []regexp.Regexp
	for _, t := range topic {
		rgx, err := regexp.Compile(t)
		if err != nil {
			return nil, err
		}
		rgxTopics = append(rgxTopics, *rgx)
	}

	return &Consumer{
		brokers:          brokers,
		regexTopics:      rgxTopics,
		consumerGroup:    &cg,
		rawClient:        c,
		incomingMessages: make(chan *shared.KafkaMessage, 100_000),
		messagesToMark:   make(chan *shared.KafkaMessage, 100_000),
		running:          atomic.Bool{},
	}, nil
}

// Start runs the Consumer.
func (c *Consumer) Start(ctx context.Context) error {
	if c.running.Swap(true) {
		return nil
	}
	c.internalCtx, c.consumerContextCancel = context.WithCancel(ctx)
	go c.consume()
	go c.recheck()
	return nil
}

func (c *Consumer) consume() {
	for c.running.Load() {
		handler := &GroupHandler{
			incomingMessages: c.incomingMessages,
			messagesToMark:   c.messagesToMark,
			running:          &c.running,
			markedMessages:   &c.markedMessages,
			consumedMessages: &c.consumedMessages,
		}

		zap.S().Infof("starting consumer with topics %v", c.actualTopics)

		if err := (*c.consumerGroup).Consume(c.internalCtx, c.actualTopics, handler); err != nil {
			// Check if the error is "no topics provided"
			if err.Error() == "no topics provided" {
				zap.S().Info("no topics provided")
				time.Sleep(shared.CycleTime * 10)
				continue
			}
			c.running.Store(false)
			zap.S().Error(err)
		}
	}
}

func (c *Consumer) recheck() {
	zap.S().Infof("starting recheck")

	var topics []string
	var err error
	for c.running.Load() {
		topics, err = c.rawClient.Topics()
		if err != nil {
			continue
		}
		zap.S().Debugf("client has %v", topics)
		var newTopics []string
		for _, name := range topics {
			for _, rgx := range c.regexTopics {
				if rgx.MatchString(name) {
					newTopics = append(newTopics, name)
					break
				}
			}
		}

		var changed bool
		if len(newTopics) != len(c.actualTopics) {
			zap.S().Infof("topics changed [fast] %v -> %v", c.actualTopics, newTopics)
			changed = true
		} else {
			for i := range newTopics {
				found := false
				for j := range c.actualTopics {
					if newTopics[i] == c.actualTopics[j] {
						found = true
						break
					}
				}
				if !found {
					changed = true
					zap.S().Infof("topics changed [slow] %v -> %v", c.actualTopics, newTopics)
					break
				}
			}
		}
		if changed {
			zap.S().Infof("topics changed from %v to %v", c.actualTopics, newTopics)
			c.running.Store(false)
			if err != nil {
				zap.S().Fatal(err)
			}
			// Wait for the consumer to stop
			time.Sleep(shared.CycleTime * 10)
			c.running.Store(true)
			c.actualTopics = newTopics
			c.consume()
			zap.S().Infof("restarted consumer with topics %v", c.actualTopics)
		} else {
			zap.S().Debugf("topics did not change")
		}
		_ = c.rawClient.RefreshMetadata()
		time.Sleep(shared.CycleTime * 50)
	}
	zap.S().Infof("stopped recheck")
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
