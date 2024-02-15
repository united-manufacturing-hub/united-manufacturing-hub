package raw

import (
	"context"
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

type ConsumerState int

const (
	ConsumerStateUnknown ConsumerState = iota
	ConsumerStateEmpty
	ConsumerStateStable
	ConsumerStatePreparingRebalance
	ConsumerStateCompletingRebalance
	ConsumerStateDead
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
	groupName             string
	groupState            ConsumerState
	externalCtx           context.Context
	runConsumerGroup      atomic.Bool
	consuming             atomic.Bool
}

// NewConsumer initializes a Consumer.
func NewConsumer(brokers, topic []string, groupName string, instanceId string) (*Consumer, error) {
	zap.S().Infof("connecting to brokers: %v", brokers)
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Group.InstanceId = instanceId
	config.Version = sarama.V2_3_0_0

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
		runConsumerGroup: atomic.Bool{},
		groupName:        groupName,
		groupState:       ConsumerStateUnknown,
	}, nil
}

func (c *Consumer) GetTopics() []string {
	return c.actualTopics
}

// Start runs the Consumer.
func (c *Consumer) Start(ctx context.Context) error {
	if c.running.Swap(true) {
		return nil
	}
	c.externalCtx = ctx
	c.internalCtx, c.consumerContextCancel = context.WithCancel(ctx)
	err := c.check()
	if err != nil {
		return err
	}
	go c.consume()
	go c.recheck()
	go c.updateState()
	return nil
}

func (c *Consumer) consume() {
	alreadyConsuming := c.consuming.Swap(true)
	if alreadyConsuming {
		zap.S().Fatalf("consume called while already consuming")
	}
	for c.running.Load() {
		handler := &GroupHandler{
			incomingMessages: c.incomingMessages,
			messagesToMark:   c.messagesToMark,
			running:          &c.runConsumerGroup,
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
			} else if strings.Contains(err.Error(), "i/o timeout") {
				zap.S().Info("i/o timeout, trying later")
				time.Sleep(shared.CycleTime * 10)
				continue
			} else if strings.Contains(err.Error(), "context canceled") {
				zap.S().Info("context canceled, trying later")
				time.Sleep(shared.CycleTime * 10)
				continue
			} else if strings.Contains(err.Error(), "EOF") {
				zap.S().Info("EOF, trying later")
				time.Sleep(shared.CycleTime * 10)
				continue
			}
			c.running.Store(false)
			zap.S().Error(err)
		}
	}
	zap.S().Infof("stopped consumer")
	c.consuming.Store(false)
}

func (c *Consumer) check() error {
	var err error
	var topics []string
	topics, err = c.rawClient.Topics()
	if err != nil {
		return err
	}
	for _, name := range topics {
		for _, rgx := range c.regexTopics {
			if rgx.MatchString(name) {
				c.actualTopics = append(c.actualTopics, name)
				break
			}
		}
	}
	return nil
}

func (c *Consumer) recheck() {
	zap.S().Infof("starting recheck")

	var topics []string
	var err error
	for !c.rawClient.Closed() {
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
			c.consumerContextCancel()
			if err != nil {
				zap.S().Fatal(err)
			}
			for c.consuming.Load() {
				time.Sleep(shared.CycleTime * 10)
				zap.S().Debugf("waiting for consumer to stop")
			}
			// Wait for the consumer to stop
			time.Sleep(shared.CycleTime * 10)
			c.running.Store(true)
			c.actualTopics = newTopics
			c.internalCtx, c.consumerContextCancel = context.WithCancel(c.externalCtx)
			go c.consume()
			zap.S().Infof("restarted consumer with topics %v", c.actualTopics)
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

func (c *Consumer) updateState() {
	adminClient, err := sarama.NewClusterAdmin(c.brokers, sarama.NewConfig())
	if err != nil {
		zap.S().Fatal(err)
	}
	var groups []*sarama.GroupDescription
	var lastRunState bool
	lastRunState = false
	for {
		groups, err = adminClient.DescribeConsumerGroups([]string{c.groupName})
		if err != nil {
			zap.S().Warnf("failed to describe consumer groups: %s", err)
			time.Sleep(shared.CycleTime * 10)
			continue
		}

		if len(groups) != 1 {
			zap.S().Warnf("expected 1 consumer group, got %d", len(groups))
			time.Sleep(shared.CycleTime * 10)
			continue
		}
		currentGroup := groups[0]

		switch currentGroup.State {
		case "Empty":
			c.groupState = ConsumerStateEmpty
		case "Stable":
			c.groupState = ConsumerStateStable
			c.runConsumerGroup.Store(true)
			lastRunState = true
		case "PreparingRebalance":
			c.groupState = ConsumerStatePreparingRebalance
			if lastRunState {
				c.runConsumerGroup.Store(false)
			}
		case "CompletingRebalance":
			c.groupState = ConsumerStateCompletingRebalance
		case "Dead":
			c.groupState = ConsumerStateDead
		default:
			c.groupState = ConsumerStateUnknown
			zap.S().Warnf("unknown consumer group state: %s", currentGroup.State)
		}

		time.Sleep(shared.CycleTime * 10)
	}
}

func (c *Consumer) GetState() ConsumerState {
	return c.groupState
}
