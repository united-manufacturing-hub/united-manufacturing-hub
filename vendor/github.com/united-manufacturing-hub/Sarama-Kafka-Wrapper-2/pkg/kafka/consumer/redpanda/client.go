package redpanda

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Consumer represents a Kafka consumer.
type Consumer struct {
	// subscribeRegexes holds compiled regular expressions to filter topics.
	subscribeRegexes []*regexp.Regexp

	// topics holds the list of Kafka topics the consumer is subscribed to.
	topics []string

	// topicsMutex provides concurrent access protection for the topics.
	topicsMutex sync.RWMutex

	// groupId represents the Kafka consumer group ID.
	groupId string

	// incomingMessages is a channel for incoming Kafka messages.
	incomingMessages chan *shared.KafkaMessage

	// messagesToMarkChan is a channel for marking messages as processed.
	messagesToMarkChan chan *shared.KafkaMessage

	// read tracks the number of messages read.
	read atomic.Uint64

	// marked tracks the number of messages marked as processed.
	marked atomic.Uint64

	// isReady indicates if the consumer is ready to consume messages.
	isReady atomic.Bool

	// config holds the Sarama consumer configuration.
	config *sarama.Config

	// brokers lists the Kafka brokers.
	brokers []string

	// client is the Sarama client for Kafka communication.
	client *sarama.Client

	// consumerGroup is the Sarama consumer group.
	consumerGroup *sarama.ConsumerGroup
}

// genIID generates an instance ID by appending a timestamp to the provided instanceId and hashing it.
func genIID(instanceId string) string {
	// Append random suffix to avoid conflicts
	// For our purposes, unix nanoseconds is good enough
	now := time.Now().UnixNano()
	x := instanceId + "-" + strconv.FormatInt(now, 10)
	// Hash the string to avoid weird characters (using SHA-256)
	hasher := sha256.New()
	hasher.Write([]byte(x))
	return hex.EncodeToString(hasher.Sum(nil))
}

// NewConsumer initializes and returns a new Consumer instance.
func NewConsumer(kafkaBrokers, subscribeRegexes []string, groupId, instanceId string) (*Consumer, error) {
	zap.S().Infof("Connecting to brokers: %v", kafkaBrokers)
	zap.S().Infof("Creating new consumer with Group ID: %s, Instance ID: %s", groupId, instanceId)
	zap.S().Infof("Subscribing to topics: %v", subscribeRegexes)

	sarama.Logger = zap.NewStdLog(zap.L())

	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second
	config.Consumer.Group.InstanceId = genIID(instanceId)
	config.Version = sarama.V2_3_0_0
	config.Metadata.RefreshFrequency = 1 * time.Minute

	c := Consumer{}
	c.subscribeRegexes = make([]*regexp.Regexp, len(subscribeRegexes))
	for i, regex := range subscribeRegexes {
		re, err := regexp.Compile(regex)
		if err != nil {
			zap.S().Errorf("Failed to compile regex: %v", err)
			return nil, err
		}
		c.subscribeRegexes[i] = re
	}
	c.groupId = groupId
	c.config = config
	c.brokers = kafkaBrokers

	zap.S().Debugf("Setting up channels")
	c.incomingMessages = make(chan *shared.KafkaMessage, 100_000)
	c.messagesToMarkChan = make(chan *shared.KafkaMessage, 100_000)

	zap.S().Debugf("Setting up initial client")
	newClient, err := sarama.NewClient(kafkaBrokers, config)
	if err != nil {
		zap.S().Errorf("Failed to create new client: %v", err)
		return nil, err
	}

	for {
		err = newClient.RefreshMetadata()
		if err != nil {
			zap.S().Errorf("Failed to refresh metadata: %v", err)
			return nil, err
		}

		var topics []string
		topics, err = newClient.Topics()
		if err != nil {
			zap.S().Errorf("Failed to retrieve topics: %v", err)
			return nil, err
		}
		zap.S().Debugf("Filtering topics")
		topics = filter(topics, c.subscribeRegexes)
		if len(topics) > 0 {
			c.topicsMutex.Lock()
			c.topics = topics
			c.topicsMutex.Unlock()
			break
		}
		zap.S().Infof("No topics found. Waiting for 1 second")
		time.Sleep(1 * time.Second)
	}
	err = newClient.Close()
	if err != nil {
		zap.S().Warnf("Failed to close initial client: %s", err)
	}

	go c.start()
	go c.refreshTopics()

	zap.S().Debugf("Consumer initialized with Group ID: %s, Instance ID: %s, Brokers: %v", groupId, instanceId, kafkaBrokers)
	return &c, nil
}

// start begins the consumption process for Kafka messages.
// If required it also re-initializes the consumer.
func (c *Consumer) start() {
	zap.S().Debugf("Starting consumer with Group ID: %s", c.groupId)
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		var topics []string
		c.topicsMutex.RLock()
		topics = make([]string, len(c.topics))
		copy(topics, c.topics)
		c.topicsMutex.RUnlock()
		if len(topics) == 0 {
			zap.S().Infof("No topics found. Waiting for 1 second")
			time.Sleep(1 * time.Second)
			continue
		}
		zap.S().Debugf("Got topics: %v", topics)

		if c.client != nil {
			if !(*c.client).Closed() {
				zap.S().Infof("Closing old client")
				err = (*c.client).Close()
				if err != nil {
					zap.S().Warnf("Failed to close client: %s", err)
				}
			}
		}
		zap.S().Debugf("Creating new client")
		var client sarama.Client
		c.config.Consumer.Group.InstanceId = genIID(c.config.Consumer.Group.InstanceId)
		zap.S().Debugf("Using instanceId %s", c.config.Consumer.Group.InstanceId)
		client, err = sarama.NewClient(c.brokers, c.config)
		c.client = &client
		if err != nil {
			zap.S().Errorf("Failed to create new client: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		zap.S().Debugf("Creating new consumer")
		consumer, err := sarama.NewConsumerGroupFromClient(c.groupId, client)
		if err != nil {
			zap.S().Errorf("Failed to create new consumer: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		c.consumerGroup = &consumer

		// Consume loop
		zap.S().Infof("Starting to consume messages")
		for {
			cgh := ConsumerGroupHandler{
				incomingMessages:   c.incomingMessages,
				messagesToMarkChan: c.messagesToMarkChan,
				ready:              &c.isReady,
				read:               &c.read,
				marked:             &c.marked,
			}
			zap.S().Debugf("CHG topics: %v", topics)
			err = consumer.Consume(ctx, topics, &cgh)
			if errors.Is(err, sarama.ErrClosedClient) {
				zap.S().Infof("Consumer closed")
				time.Sleep(5 * time.Second)
				break
			} else if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				zap.S().Infof("Consumer group closed")
				time.Sleep(5 * time.Second)
				break
			} else if err != nil {
				zap.S().Errorf("Consumer error: %v", err)
				time.Sleep(1 * time.Second)
			}
			if ctx.Err() != nil {
				zap.S().Infof("Context closed")
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		zap.S().Debugf("Consumer start loop ended for Group ID: %s", c.groupId)
	}
}

// refreshTopics periodically updates the list of topics the consumer subscribes to.
func (c *Consumer) refreshTopics() {

	ticker := time.NewTicker(5 * time.Second)
	for {
		zap.S().Debugf("Starting topic refresh for consumer with Group ID: %s", c.groupId)
		<-ticker.C
		if c.client == nil {
			zap.S().Debugf("Client not ready")
			continue
		}
		zap.S().Debugf("Refreshing metadata")

		err := (*c.client).RefreshMetadata()
		if err != nil {
			zap.S().Errorf("Error refreshing metadata: %v", err)
			continue
		}

		topics, err := (*c.client).Topics()
		if err != nil {
			zap.S().Errorf("Error getting topics: %v", err)
			continue
		}

		topics = filter(topics, c.subscribeRegexes)
		c.topicsMutex.RLock()
		compare := slices.Compare(c.topics, topics)
		c.topicsMutex.RUnlock()
		if compare == 0 {
			zap.S().Infof("No change in topics")
			continue
		}
		c.topicsMutex.Lock()
		zap.S().Infof("Detected topic change. Old topics: %v, New topics: %v", c.topics, topics)
		c.topics = topics
		c.topicsMutex.Unlock()

		if c.consumerGroup != nil {
			err = (*c.consumerGroup).Close()
			if err != nil {
				zap.S().Warnf("Failed to close consumer group: %s", err)
			}
		}
		if c.client != nil {
			err = (*c.client).Close()
			if err != nil {
				zap.S().Warnf("Failed to close client: %s", err)
			}
		}
		zap.S().Debugf("Refresh loop ended")
		// Reset the ticker to avoid a burst of refreshes
		ticker.Reset(5 * time.Second)
		zap.S().Debugf("Topic refresh loop ended for Group ID: %s", c.groupId)
	}
}

// GetStats returns consumed message counts.
func (c *Consumer) GetStats() (uint64, uint64) {
	return c.marked.Load(), c.read.Load()
}

// GetTopics returns the topics that the consumer is subscribed to.
func (c *Consumer) GetTopics() []string {
	c.topicsMutex.RLock()
	topics := make([]string, len(c.topics))
	copy(topics, c.topics)
	c.topicsMutex.RUnlock()
	return topics
}

// GetMessage returns the next message from the consumer.
func (c *Consumer) GetMessage() *shared.KafkaMessage {
	return <-c.incomingMessages
}

// GetMessages returns the channel of messages from the consumer.
func (c *Consumer) GetMessages() <-chan *shared.KafkaMessage {
	return c.incomingMessages
}

// MarkMessage marks a message as processed.
func (c *Consumer) MarkMessage(message *shared.KafkaMessage) {
	c.messagesToMarkChan <- message
}

// MarkMessages marks a slice of messages as processed.
func (c *Consumer) MarkMessages(messages []*shared.KafkaMessage) {
	for _, message := range messages {
		c.messagesToMarkChan <- message
	}
}

// IsReady returns whether the consumer is ready to consume messages.
func (c *Consumer) IsReady() bool {
	return c.isReady.Load()
}

// filter applies regular expression filters to a list of topics and returns the filtered list.
func filter(topics []string, regexes []*regexp.Regexp) []string {
	filtered := make(map[string]bool)
	for _, topic := range topics {
		for _, re := range regexes {
			if re.MatchString(topic) {
				filtered[topic] = true
				break
			}
		}
	}
	result := make([]string, 0, len(filtered))
	for topic := range filtered {
		result = append(result, topic)
	}
	zap.S().Debugf("Filtered topics: %v to %v", topics, result)
	slices.Sort(result)
	return result
}
