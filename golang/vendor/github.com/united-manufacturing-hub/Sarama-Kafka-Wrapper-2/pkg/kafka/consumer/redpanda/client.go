package redpanda

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Consumer struct {
	rawClient               sarama.Client
	httpClients             []string
	consumerGroup           *sarama.ConsumerGroup
	regexTopics             []regexp.Regexp
	actualTopics            []string
	actualTopicsLock        sync.RWMutex
	running                 atomic.Bool
	cgContext               context.Context
	cgCncl                  context.CancelFunc
	incomingMessages        chan *shared.KafkaMessage
	messagesToMark          chan *shared.KafkaMessage
	markedMessages          atomic.Uint64
	consumedMessages        atomic.Uint64
	consumerShutdownChannel chan bool
	groupName               string
	greeter                 bool
}

// GetStats returns marked and consumed message counts.
func (c *Consumer) GetStats() (uint64, uint64) {
	return c.markedMessages.Load(), c.consumedMessages.Load()
}

func NewConsumer(kafkaBrokers, httpBrokers, subscribeRegexes []string, groupName, instanceId string, greeter bool) (*Consumer, error) {
	zap.S().Infof("connecting to brokers: %v", kafkaBrokers)
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second
	config.Consumer.Group.InstanceId = instanceId
	config.Version = sarama.V2_3_0_0

	c, err := sarama.NewClient(kafkaBrokers, config)
	if err != nil {
		return nil, err
	}
	zap.S().Infof("connected to brokers: %v", kafkaBrokers)

	var rgxTopics []regexp.Regexp
	for _, t := range subscribeRegexes {
		rgx, err := regexp.Compile(t)
		if err != nil {
			return nil, err
		}
		rgxTopics = append(rgxTopics, *rgx)
	}
	return &Consumer{
		rawClient:               c,
		httpClients:             httpBrokers,
		regexTopics:             rgxTopics,
		actualTopics:            []string{},
		running:                 atomic.Bool{},
		groupName:               groupName,
		markedMessages:          atomic.Uint64{},
		consumerShutdownChannel: make(chan bool, 512), // This is oversized, but it's better to be safe than sorry
		incomingMessages:        make(chan *shared.KafkaMessage, 100_000),
		messagesToMark:          make(chan *shared.KafkaMessage, 100_000),
		greeter:                 greeter,
	}, nil
}

func (c *Consumer) GetTopics() []string {
	return c.actualTopics
}

func (c *Consumer) Start() error {
	if c.running.Swap(true) {
		return nil
	}
	c.cgContext, c.cgCncl = context.WithCancel(context.Background())

	go c.generateTopics()
	go c.consumer()
	go c.reporter()
	return nil
}

func (c *Consumer) generateTopics() {
	zap.S().Debugf("Started topic generator")

	var httpClient http.Client
	httpClient.Timeout = 5 * time.Second

	ticker := time.NewTicker(5 * time.Second)

	for c.running.Load() {
		<-ticker.C

		clients := c.httpClients
		topics := make(map[string]bool)
		for _, client := range clients {
			url := fmt.Sprintf("http://%s/topics", client)
			zap.S().Infof("fetching topics from %s", url)
			response, err := httpClient.Get(url)
			if err != nil {
				zap.S().Errorf("failed to fetch topics from %s: %v", url, err)
				continue
			}
			zap.S().Debugf("Finished http request")

			// Parse as []string from JSON
			var topicsX []string
			err = json.NewDecoder(response.Body).Decode(&topicsX)
			if err != nil {
				zap.S().Errorf("failed to parse topics from %s: %v", url, err)
				continue
			}
			err = response.Body.Close()
			if err != nil {
				zap.S().Errorf("failed to close response body from %s: %v", url, err)
				continue
			}

			// Add topics to map
			for _, topic := range topicsX {
				topics[topic] = true
			}
			zap.S().Debugf("Fetched %d topics from remote", len(topics))
		}

		// Filter topics by regex
		var actualTopics []string
		for topic := range topics {
			for _, rgx := range c.regexTopics {
				if rgx.MatchString(topic) {
					actualTopics = append(actualTopics, topic)
					break
				}
			}
		}
		zap.S().Debugf("After regex check we have %d topics", len(actualTopics))

		// Check if topics changed
		c.actualTopicsLock.RLock()
		changed := false
		for _, topic := range actualTopics {
			found := false
			for _, topic2 := range c.actualTopics {
				if topic == topic2 {
					found = true
					break
				}
			}
			if !found {
				changed = true
				if c.greeter {
					c.incomingMessages <- &shared.KafkaMessage{
						Topic: topic,
						Value: []byte(""),
					}
				}
				break
			}
		}
		c.actualTopicsLock.RUnlock()

		if changed {
			zap.S().Infof("topics changed: %v", actualTopics)
			c.actualTopicsLock.Lock()
			c.actualTopics = actualTopics
			c.actualTopicsLock.Unlock()
			zap.S().Debugf("updated actual topics")
			c.cgCncl()
			shutdown(c.consumerShutdownChannel)
			c.cgContext, c.cgCncl = context.WithCancel(context.Background())
		} else {
			zap.S().Debugf("topics unchanged")
		}
		zap.S().Debugf("Finished topic generator")
	}
	zap.S().Debugf("Goodbye topic generator")
}

func (c *Consumer) consumer() {
	zap.S().Debugf("Started consumer")
	ticker := time.NewTicker(shared.CycleTime)
	for c.running.Load() {
		<-ticker.C
		zap.S().Debugf("Getting topics")
		c.actualTopicsLock.RLock()
		topicClone := make([]string, len(c.actualTopics))
		copy(topicClone, c.actualTopics)
		c.actualTopicsLock.RUnlock()

		if len(topicClone) == 0 {
			zap.S().Debugf("No topics for consume, trying later")
			time.Sleep(5 * time.Second)
			continue
		}

		zap.S().Debugf("Create handler")
		c.consumerShutdownChannel = make(chan bool, 512) // This is oversized, but it's better to be safe than sorry
		handler := &GroupHandler{
			incomingMessages: c.incomingMessages,
			messagesToMark:   c.messagesToMark,
			markedMessages:   &c.markedMessages,
			consumedMessages: &c.consumedMessages,
			shutdownChannel:  c.consumerShutdownChannel,
		}
		zap.S().Debugf("Create consumer group")
		c.createConsumerGroup()

		zap.S().Debugf("Beginning consume loop")
		if err := (*c.consumerGroup).Consume(c.cgContext, topicClone, handler); err != nil {
			// Check if the error is "no topics provided"
			if err.Error() == "no topics provided" {
				zap.S().Info("no topics provided")
			} else if strings.Contains(err.Error(), "i/o timeout") {
				zap.S().Info("i/o timeout, trying later")
			} else if strings.Contains(err.Error(), "context canceled") {
				zap.S().Info("context canceled, trying later")
			} else if strings.Contains(err.Error(), "EOF") {
				zap.S().Info("EOF, trying later")
			} else {
				zap.S().Errorf("failed to consume: %v", err)
			}
		}
		zap.S().Debugf("End consume loop")
	}
	zap.S().Debugf("Goodbye consumer")
}

// Close terminates the Consumer.
func (c *Consumer) Close() error {
	zap.S().Info("closing consumer")
	if !c.running.Swap(false) {
		zap.S().Info("consumer already closed")
		return nil
	}
	closeTimeout := 5 * time.Second
	select {
	case <-time.After(closeTimeout):
		zap.S().Warnf("failed to close consumer within %s", closeTimeout)
	case err := <-func() chan error {
		c.cgCncl()
		err := (*c.consumerGroup).Close()
		chanX := make(chan error, 1)
		chanX <- err
		return chanX
	}():
		return err
	}

	return nil
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

func (c *Consumer) createConsumerGroup() {
	var err error
	if c.consumerGroup != nil {
		zap.S().Debugf("Closing existing consumer group")
		err = (*c.consumerGroup).Close()
		if err != nil {
			zap.S().Errorf("Failed to close existing consumer group: %s", err)
		}
	}
	zap.S().Debugf("Refreshing metadata")
	err = c.rawClient.RefreshMetadata()
	if err != nil {
		zap.S().Errorf("Failed to refresh metadata: %s", err)
	}
	zap.S().Debugf("Creating consumer group: %v", c.groupName)
	var cg sarama.ConsumerGroup
	cg, err = sarama.NewConsumerGroupFromClient(c.groupName, c.rawClient)
	if err != nil {
		zap.S().Fatalf("Failed to create consumer group: %v", err)
	}
	zap.S().Debugf("Created consumer group")
	c.consumerGroup = &cg

}

func (c *Consumer) reporter() {
	ticker10Seconds := time.NewTicker(10 * time.Second)
	for c.running.Load() {
		<-ticker10Seconds.C
		marked, consumed := c.GetStats()
		zap.S().Infof("marked: %d, consumed: %d", marked, consumed)
		zap.S().Info("Incoming messages channel (%d/%d), Marked messages channel (%d/%d)",
			len(c.incomingMessages), cap(c.incomingMessages),
			len(c.messagesToMark), cap(c.messagesToMark))

		if c.consumerGroup != nil {
			for err := range (*c.consumerGroup).Errors() {
				zap.S().Debugf("Consumer group error: %v", err)
			}
		}
	}
}

func shutdown(c chan bool) {
	for {
		select {
		case c <- true:
		default:
			return
		}
	}
}
