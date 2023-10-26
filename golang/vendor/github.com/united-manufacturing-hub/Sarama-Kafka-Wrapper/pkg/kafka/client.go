package kafka

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"golang.org/x/sync/syncmap"
)

var sentMessages atomic.Uint64
var receivedMessages atomic.Uint64
var sentBytesApprox atomic.Uint64
var receivedBytesApprox atomic.Uint64

// GetKafkaStats returns the number of sent and received messages and approximate bytes.
func GetKafkaStats() (sent uint64, received uint64, sentBytesA uint64, recvBytesA uint64) {
	return sentMessages.Load(), receivedMessages.Load(), sentBytesApprox.Load(), receivedBytesApprox.Load()
}

type Client struct {
	admin                      sarama.ClusterAdmin
	consumer                   sarama.ConsumerGroup
	producer                   sarama.AsyncProducer
	topicRegex                 *regexp.Regexp
	consumerMessageQueue       chan Message
	producerMessageQueue       chan Message
	topics                     sync.Map
	consumerGroupId            string
	consumerGroupHandlers      []*ConsumerGroupHandler
	lastCheck                  atomic.Int64
	topicCheckSum              atomic.Uint64
	consumerGroupHandlersLock  sync.RWMutex
	newTopicsPartitions        int32
	open                       atomic.Bool
	readyToRead                atomic.Bool
	closing                    atomic.Bool
	hasBroker                  atomic.Bool
	automark                   atomic.Bool
	newTopicsReplicationFactor int16
}

// NewClientOptions are the options for creating a new kafka client.
// ListenTopicRegex is the regex to match topics to listen to.
// ConsumerName is the name of the consumer group (group.id).
// Brokers is the list of brokers to connect to.
// StartOffset is the offset to start consuming from.
// Partitions is the number of partitions to create for new topics.
// ReplicationFactor is the replication factor to use for new topics.
// EnableTLS enables TLS for the connection.
// SenderTag controls the sender tagging feature.
// ClientID is the client ID to use for the connection, only relevant for debugging.
// AutoCommit enables auto-commit (Default: true) [Note: AutoCommit only commits Marked Messages. You shouldn't need to set this to false]
// OpenDeadLine is the deadline, until connection to the brokers must be established.
// ProducerReturnSuccesses enables the success output channel of back to the user.
// TransactionalID is required for transaction of producers, e.g.,BeginTxn().
// AutoMark enabled automatic marking of messages as consumed. [Set this to false if you want to manually mark messages as consumed]
type NewClientOptions struct {
	ListenTopicRegex        *regexp.Regexp
	SenderTag               SenderTag
	ConsumerGroupId         string
	TransactionalID         string
	ClientID                string
	Brokers                 []string
	StartOffset             int64
	OpenDeadLine            time.Duration
	Partitions              int32
	ReplicationFactor       int16
	EnableTLS               bool
	AutoCommit              bool
	ProducerReturnSuccesses bool
	AutoMark                bool
}

// SenderTag controls the sender tagging feature.
// Enabled enables the feature.
// OverwriteSerialNumber overwrites the serial number with the given value,
// else the SERIAL_NUMBER env variable is used.
// OverwriteMicroserviceName overwrites the microservice name with the given value,
// else the MICROSERVICE_NAME env variable is used.
type SenderTag struct {
	OverwriteSerialNumber     *string
	OverwriteMicroserviceName *string
	Enabled                   bool
}

var topicCache = syncmap.Map{}
var lastTopicChange = atomic.Int64{}

var enableTagging = false
var SerialNumber, snErr = env.GetAsString("SERIAL_NUMBER", false, "")
var MicroserviceName, mnErr = env.GetAsString("MICROSERVICE_NAME", false, "")

func NewKafkaClient(opts *NewClientOptions) (client *Client, err error) {
	if lastTopicChange.Load() == 0 {
		lastTopicChange.Store(time.Now().UnixNano())
	}
	if opts == nil {
		return nil, errors.New("no options provided")
	}

	if opts.SenderTag.Enabled {
		enableTagging = true
		if opts.SenderTag.OverwriteSerialNumber != nil {
			SerialNumber = *opts.SenderTag.OverwriteSerialNumber
		} else if snErr != nil || SerialNumber == "" {
			return nil, errors.New("sender tagging is enabled but no serial number is set")
		}
		if opts.SenderTag.OverwriteMicroserviceName != nil {
			MicroserviceName = *opts.SenderTag.OverwriteMicroserviceName
		} else if mnErr != nil || MicroserviceName == "" {
			return nil, errors.New("sender tagging is enabled but no microservice name is set")
		}
	}

	client = &Client{}
	client.consumerGroupHandlersLock = sync.RWMutex{}
	client.consumerGroupHandlersLock.Lock()
	client.consumerGroupHandlers = make([]*ConsumerGroupHandler, 0, 1)
	client.consumerGroupHandlersLock.Unlock()
	client.automark = atomic.Bool{}
	client.automark.Store(opts.AutoMark)

	config := sarama.NewConfig()
	if opts.ClientID != "" {
		config.ClientID = opts.ClientID
		config.Consumer.Group.InstanceId = opts.ClientID
	}
	config.Producer.Return.Errors = true
	if opts.ConsumerGroupId != "" {
		client.consumerGroupId = opts.ConsumerGroupId //nolint:govet
	}
	config.Version = sarama.MaxVersion

	switch opts.StartOffset {
	case sarama.OffsetNewest:
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	case sarama.OffsetOldest:
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
	default:
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	// Set TLS
	config.Net.TLS.Enable = opts.EnableTLS
	if !config.Net.TLS.Enable {
		config.Net.TLS.Config = nil
	}

	config.Producer.Return.Successes = opts.ProducerReturnSuccesses

	config.Producer.Transaction.ID = opts.TransactionalID
	config.Producer.Idempotent = opts.TransactionalID != ""
	if config.Producer.Idempotent {
		config.Producer.RequiredAcks = sarama.WaitForAll
		config.Net.MaxOpenRequests = 1
	}

	if !opts.AutoCommit {
		config.Consumer.Offsets.AutoCommit.Enable = opts.AutoCommit
	}

	config.Version = sarama.V2_3_0_0

	client.consumer, err = sarama.NewConsumerGroup(opts.Brokers, opts.ConsumerGroupId, config)
	if err != nil {
		zap.S().Errorf("Error creating consumer group client: %v", err)
		return nil, err
	}

	client.producer, err = sarama.NewAsyncProducer(opts.Brokers, config)

	if err != nil {
		zap.S().Errorf("Error creating producer client: %v", err)
		return nil, err
	}

	client.admin, err = sarama.NewClusterAdmin(opts.Brokers, config)

	if err != nil {
		zap.S().Errorf("Error creating admin client: %v", err)
		return nil, err
	}

	client.topicRegex = opts.ListenTopicRegex
	client.open.Store(true)
	client.producerMessageQueue = make(chan Message, 1000)

	client.consumerMessageQueue = make(chan Message)

	client.readyToRead.Store(false)
	client.newTopicsPartitions = opts.Partitions
	client.newTopicsReplicationFactor = opts.ReplicationFactor

	if client.newTopicsReplicationFactor <= 0 {
		zap.S().Fatalf("Replication factor must be greater than 0. Got %d", client.newTopicsReplicationFactor)
	}
	if client.newTopicsPartitions <= 0 {
		zap.S().Fatalf("Partitions must be greater than 0. Got %d", client.newTopicsPartitions)
	}

	client.populateTopics(opts.OpenDeadLine)

	go client.errorReader()
	go client.messageSender()
	go client.reloadTopics()
	go client.refreshAllTopics()
	go client.subscriber()
	return client, nil
}

func (c *Client) GetQueueLength() int {
	if c.producerMessageQueue == nil {
		return 0
	}
	return len(c.producerMessageQueue)
}

func (c *Client) Close() error {
	if !c.open.Load() || c.closing.Load() {
		return nil
	}
	c.readyToRead.Store(false)
	c.closing.Store(true)

	err := c.consumer.Close()
	if err != nil {
		return err
	}

	ctx10Sec, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

emptyLoop:
	for len(c.producerMessageQueue) > 0 || len(c.producer.Input()) > 0 {

		select {
		case <-ctx10Sec.Done():
			zap.S().Errorf("Failed to empty send queue in time. %d messages left", len(c.producerMessageQueue))
			break emptyLoop
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	err = c.producer.Close()
	if err != nil {
		return err
	}
	err = c.admin.Close()
	if err != nil {
		return err
	}
	c.open.Store(false)

	return nil
}

func (c *Client) TopicCreator(topic string) (err error) {
	if !c.open.Load() {
		return errors.New("client is closed")
	}
	var exists bool
	exists, err = c.checkIfTopicExists(topic)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	topicCache.Store(topic, false)
	err = c.admin.CreateTopic(topic, &sarama.TopicDetail{
		NumPartitions:     c.newTopicsPartitions,
		ReplicationFactor: c.newTopicsReplicationFactor,
	}, false)
	return err
}

func (c *Client) checkIfTopicExists(topic string) (exists bool, err error) {
	initialized, ok := topicCache.Load(topic)
	if !ok {
		var topics map[string]sarama.TopicDetail
		topics, err = c.admin.ListTopics()
		if err != nil {
			return false, err
		}
		_, exists = topics[topic]
		topicCache.Store(topic, exists)
		lastTopicChange.Store(time.Now().UnixNano())
		return exists, nil
	} else if !initialized.(bool) {
		var topics map[string]sarama.TopicDetail
		topics, err = c.admin.ListTopics()
		if err != nil {
			return false, err
		}
		_, exists = topics[topic]
		topicCache.Store(topic, exists)
		lastTopicChange.Store(time.Now().UnixNano())
		return exists, nil
	}

	return initialized.(bool), nil
}

func (c *Client) messageSender() {
	zap.S().Infof("Starting message sender")
	for c.open.Load() {
		select {
		case msg := <-c.producerMessageQueue:
			// Check if the topic exists
			exists, err := c.checkIfTopicExists(msg.Topic)
			if err != nil {
				zap.S().Errorf("Error checking if topic exists: %v", err)
				// re-queue message
				c.producerMessageQueue <- msg
				time.Sleep(5 * time.Second)
				continue
			}
			if !exists {
				zap.S().Errorf("Topic %s does not exist yet", msg.Topic)
				// re-queue message
				c.producerMessageQueue <- msg
				time.Sleep(1 * time.Second)
				continue
			}

			var records []sarama.RecordHeader
			records = make([]sarama.RecordHeader, 0, len(msg.Header))
			for s, bytes := range msg.Header {
				records = append(records, sarama.RecordHeader{Key: []byte(s), Value: bytes})
			}

			err = addTagsToHeaders(&records)
			if err != nil {
				zap.S().Errorf("Error adding tags to headers: %v", err)
				continue
			}
			// Check if closing before sending message
			if c.closing.Load() || !c.open.Load() || c.producer == nil {

				// re-queue message
				if c.producerMessageQueue != nil {
					c.producerMessageQueue <- msg
				}
				continue
			}

			c.producer.Input() <- &sarama.ProducerMessage{
				Topic:    msg.Topic,
				Key:      sarama.ByteEncoder(msg.Key),
				Headers:  records,
				Value:    sarama.ByteEncoder(msg.Value),
				Metadata: sarama.MetadataRequest{AllowAutoTopicCreation: true},
			}
			// We don't need to worry about overflow here
			// Even with 1000 messages per second, it would take ~116988483 years to overflow
			sentMessages.Add(1)
			sentBytesApprox.Add(uint64(len(msg.Value)))
		case <-time.After(5 * time.Second):
			continue
		}
	}
	zap.S().Infof("Stopping message sender")
}

func (c *Client) EnqueueMessage(msg Message) (err error) {
	if !c.open.Load() {
		return errors.New("client is closed")
	}

	err = c.TopicCreator(msg.Topic)
	if err != nil {
		return err
	}

	// try insert to producerMessageQueue, if not full, insert, else return error
	select {
	case c.producerMessageQueue <- msg:
		return nil
	default:
		return errors.New("message queue is full")
	}
}

func (c *Client) errorReader() {
	for c.open.Load() {
		select {
		case err := <-c.producer.Errors():
			if err != nil {
				zap.S().Errorf("Error writing message: %v", err)
			}
		// Recheck c.open every 5 seconds
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (c *Client) getTopicsForRegex() []string {
	if !c.open.Load() {
		return nil
	}
	if c.topicRegex == nil {
		return nil
	}

	var subTopics []string
	topicCache.Range(func(key, value interface{}) bool {
		if c.topicRegex.MatchString(key.(string)) {
			subTopics = append(subTopics, key.(string))
		}
		return true
	})

	// sort topics
	sort.Strings(subTopics)

	return subTopics
}

func (c *Client) reloadTopics() {
	c.lastCheck.Store(0)
	for c.open.Load() {
		if lastTopicChange.Load() > c.lastCheck.Load() {
			c.lastCheck.Store(time.Now().UnixNano())

			subTopics := c.getTopicsForRegex()
			if subTopics == nil {
				time.Sleep(5 * time.Second)
				continue
			}
			// Generate hash by iterating over topics and appending using xxhash64
			hasher := xxh3.New()
			for _, topic := range subTopics {
				_, err := hasher.WriteString(topic)
				if err != nil {
					return
				}
			}
			checkSum := hasher.Sum64()

			if checkSum != c.topicCheckSum.Load() {
				for _, topic := range subTopics {
					c.topics.Store(topic, true)
				}

				c.topics.Range(func(key, value interface{}) bool {
					if !contains(subTopics, key.(string)) {
						c.topics.Delete(key)
					}
					return true
				})

				c.topicCheckSum.Store(checkSum)

			}
		} else {
			time.Sleep(5 * time.Second)
		}
	}
}

func contains[T comparable](a []T, x T) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func (c *Client) ChangeSubscribedTopics(newTopicRegex *regexp.Regexp) {
	c.topicRegex = newTopicRegex
	c.lastCheck.Store(0)
}

func (c *Client) subscriber() {
	var cancel context.CancelFunc
	var ctx context.Context
	for c.open.Load() {
		c.readyToRead.Store(false)
		if cancel != nil {
			cancel()
		}
		if getSyncMapAvgSize(&c.topics) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}
		ctx, cancel = context.WithCancel(context.Background())
		go c.gSub(ctx, cancel)
		c.readyToRead.Store(true)
		time.Sleep(5 * time.Second)
	}
	if cancel != nil {
		cancel()
	}
}

func getSyncMapAvgSize(m *sync.Map) int {
	var size int
	m.Range(func(key, value interface{}) bool {
		size++
		return true
	})
	return size
}

func (c *Client) gSub(ctx context.Context, cncl context.CancelFunc) {
	consumerGroup, err := NewConsumerGroupHandler(c.consumerMessageQueue, c.automark.Load())
	if err != nil {
		zap.S().Errorf("Error creating consumer group handler: %v", err)
		cncl()
		return
	}
	topics := make([]string, 0, getSyncMapAvgSize(&c.topics))
	c.topics.Range(func(key, value interface{}) bool {
		topics = append(topics, key.(string))
		return true
	})

	c.consumerGroupHandlersLock.Lock()
	c.consumerGroupHandlers = append(c.consumerGroupHandlers, consumerGroup.(*ConsumerGroupHandler))
	c.consumerGroupHandlersLock.Unlock()

	err = c.consumer.Consume(ctx, topics, consumerGroup)

	if err != nil {
		zap.S().Errorf("Error consuming topics: %v", err)
		cncl()
	}
}

func (c *Client) GetMessages() <-chan Message {
	return c.consumerMessageQueue
}

func (c *Client) Ready() bool {
	return c.hasBroker.Load()
}

func (c *Client) Closed() bool {
	return !c.open.Load()
}

func (c *Client) populateTopics(deadLine time.Duration) {
	if !c.open.Load() {
		zap.S().Errorf("Client is closed, can't populate topics")
		return
	}
	if deadLine <= 0 {
		zap.S().Errorf("Invalid deadline: %v, using default (1 Minute)", deadLine)
		deadLine = time.Minute
	}
	now := time.Now()
	topics, err := c.admin.ListTopics()
	if err != nil {
		if time.Since(now) > deadLine {
			zap.S().Errorf("Error listing topics: %v", err)
			err = c.Close()
			if err != nil {
				zap.S().Errorf("Error closing client: %v", err)
			}
			return
		}
		zap.S().Errorf("Error listing topics: %v, retrying in 5 seconds", err)
		time.Sleep(5 * time.Second)
		c.populateTopics(deadLine - time.Since(now))
	}
	c.hasBroker.Store(true)
	for topic := range topics {
		topicCache.Store(topic, true)
	}
}

func (c *Client) refreshAllTopics() {
	var errs int
	for c.open.Load() {
		time.Sleep(5 * time.Second)
		topics, err := c.admin.ListTopics()
		if err != nil {
			zap.S().Errorf("Error listing topics (%d): %v", errs, err)
			errs++
			if errs > 5 {
				err = c.Close()
				if err != nil {
					zap.S().Errorf("Error closing client: %v", err)
				}
				return
			}
		}
		for topic := range topics {
			topicCache.Store(topic, true)
		}
		lastTopicChange.Store(time.Now().UnixNano())
	}
}

func (c *Client) MarkMessage(msg *Message) error {
	if !c.open.Load() {
		return errors.New("client is closed")
	}
	if c.automark.Load() {
		return nil
	}
	c.consumerGroupHandlersLock.RLock()
	if c.consumerGroupHandlers == nil || len(c.consumerGroupHandlers) == 0 {
		c.consumerGroupHandlersLock.RUnlock()
		return errors.New("no consumer group handlers")
	}
	markerrors := make([]error, 0, len(c.consumerGroupHandlers))
	for _, handler := range c.consumerGroupHandlers {
		markerrors = append(markerrors, handler.MarkMessage(MessageToConsumerMessage(msg), ""))
	}
	c.consumerGroupHandlersLock.RUnlock()

	// Cleanup closed handlers
	c.consumerGroupHandlersLock.Lock()
	toRemove := make([]int, 0, len(c.consumerGroupHandlers))
	for i := 0; i < len(c.consumerGroupHandlers); i++ {
		if c.consumerGroupHandlers[i].closed.Load() {
			toRemove = append(toRemove, i)
		}
	}
	for i := len(toRemove) - 1; i >= 0; i-- {
		c.consumerGroupHandlers = append(c.consumerGroupHandlers[:toRemove[i]], c.consumerGroupHandlers[toRemove[i]+1:]...)
	}
	c.consumerGroupHandlersLock.Unlock()

	// Check if at least one mark was successful
	var errX error
	for _, err := range markerrors {
		if err == nil {
			return nil
		}
		errX = err
	}

	return errX
}

type ConsumerGroupHandler struct {
	session  sarama.ConsumerGroupSession
	queue    chan Message
	closed   atomic.Bool
	automark bool
}

func (c *ConsumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	c.session = session
	c.closed.Store(false)
	zap.S().Debugf("Setup new consumer group session for MemberId: %s, GenerationId: %d", session.MemberID(), session.GenerationID())
	return nil
}

func (c *ConsumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	c.closed.Store(true)
	c.session = nil
	session.Commit()
	zap.S().Debugf("Cleaned up consumer group session for MemberId: %s, GenerationId: %d", session.MemberID(), session.GenerationID())
	return nil
}

func (c *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	messages := 0
	for message := range claim.Messages() {
		// Check if the message is valid
		if message == nil || len(message.Topic) == 0 {
			continue
		}
		msg := Message{
			Topic:     message.Topic,
			Value:     message.Value,
			Key:       message.Key,
			Partition: message.Partition,
			Offset:    message.Offset,
		}
		if message.Headers != nil {
			msg.Header = make(map[string][]byte)
			for _, header := range message.Headers {
				msg.Header[string(header.Key)] = header.Value
			}
		}
		c.queue <- msg
		if c.automark {
			session.MarkMessage(message, "")
		}
		// We don't need to worry about overflow here
		// Even with 1000 messages per second, it would take ~116988483 years to overflow
		receivedMessages.Add(1)
		receivedBytesApprox.Add(uint64(len(message.Value)))
		messages++
	}

	return nil
}

// MarkMessage marks a message as consumed.
func (c *ConsumerGroupHandler) MarkMessage(msg *sarama.ConsumerMessage, metadata string) error {
	if c.session == nil {
		return errors.New("session is nil")
	}
	c.session.MarkMessage(msg, metadata)
	zap.S().Debugf("Marked message with offset %d", msg.Offset)
	return nil
}

// Commit commits all marked messages to the broker.
// This is a blocking operation.
func (c *ConsumerGroupHandler) Commit() error {
	if c.session == nil {
		return errors.New("session is nil")
	}
	c.session.Commit()
	return nil
}

func NewConsumerGroupHandler(queue chan Message, automark bool) (sarama.ConsumerGroupHandler, error) {
	if queue == nil {
		return nil, errors.New("queue is nil")
	}
	return &ConsumerGroupHandler{
		session:  nil,
		automark: automark,
		queue:    queue,
		closed:   atomic.Bool{},
	}, nil
}

// Producer
func (c *Client) BeginTxn() error {
	return c.producer.BeginTxn()
}

func (c *Client) AddMessageToTxn(msg *Message) error {
	consumerMsg := MessageToConsumerMessage(msg)
	return c.producer.AddMessageToTxn(consumerMsg, c.consumerGroupId, nil)
}

func (c *Client) AbortTxn() error {
	return c.producer.AbortTxn()
}

func (c *Client) CommitTxn() error {
	return c.producer.CommitTxn()
}

func (c *Client) GetProducerSuccessesChannel() <-chan *sarama.ProducerMessage {
	return c.producer.Successes()
}

func (c *Client) GetProducerErrorsChannel() <-chan *sarama.ProducerError {
	return c.producer.Errors()
}

// Consumer
func (c *Client) GetConsumerErrorsChannel() <-chan error {
	return c.consumer.Errors()
}
