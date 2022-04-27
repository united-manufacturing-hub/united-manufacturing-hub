//go:build kafkaALO
// +build kafkaALO

package internal

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

// KafkaCommits is a counter for the number of commits done (to the db), this is used for stats only
var KafkaCommits = float64(0)

// KafkaMessages is a counter for the number of messages processed, this is used for stats only
var KafkaMessages = float64(0)

// KafkaPutBacks is a counter for the number of messages returned to kafka, this is used for stats only
var KafkaPutBacks = float64(0)

// KafkaConfirmed is a counter for the number of messages confirmed to kafka, this is used for stats only
var KafkaConfirmed = float64(0)

// NearMemoryLimit is set to true if 90% of the allowed memory is allocated
var NearMemoryLimit = false

type KafkaKey struct {
	Putback *KafkaPutback `json:"KafkaPutback,omitempty"`
}

type KafkaPutback struct {
	FirstTsMS int64  `json:"FirstTsMs"`
	LastTsMS  int64  `json:"LastTsMs"`
	Amount    int64  `json:"Amount"`
	Reason    string `json:"Reason,omitempty"`
	Error     string `json:"Error,omitempty"`
}

type PutBackChanMsg struct {
	Msg         *kafka.Message
	Reason      string
	ErrorString *string
}

var ShutdownMainChan = make(chan bool)
var KafkaShuttingDown bool
var KafkaShutdownPutback bool

// KafkaProcessQueue processes the kafka queue and sends the messages to the processorChannel.
// It uses topic as regex for subscribing to kafka topics.
// If the putback channel is full, it will block until the channel is free.
func KafkaProcessQueue(identifier string, topic string, processorChannel chan *kafka.Message, kafkaConsumer *kafka.Consumer, putBackChannel chan PutBackChanMsg) {
	zap.S().Debugf("%s Starting Kafka consumer for topic %s", identifier, topic)
	err := kafkaConsumer.Subscribe(topic, nil)
	if err != nil {
		zap.S().Errorf("%s Failed to subscribe to topic %s: %s", identifier, topic, err)
		panic(err)
	}

	for !KafkaShuttingDown {
		if len(putBackChannel) > 100 {
			// We have too many CountMessagesToCommitLater in the put back channel, so we need to wait for some to be processed
			zap.S().Debugf("%s Waiting for put back channel to empty: %d", identifier, len(putBackChannel))
			time.Sleep(OneSecond)
			continue
		}

		if NearMemoryLimit {
			time.Sleep(OneSecond)
			continue
		}

		var msg *kafka.Message
		// Wait for new messages
		// This has a timeout, allowing KafkaShuttingDown to be checked
		msg, err = kafkaConsumer.ReadMessage(5000)
		if err != nil {
			// This is fine, and expected behaviour
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				// Sleep to reduce CPU usage
				time.Sleep(OneSecond)
				continue
			} else if err.(kafka.Error).Code() == kafka.ErrUnknownTopicOrPart {
				// This will occur when no topic for the regex is available !
				zap.S().Errorf("%s Unknown topic or partition: %s", identifier, err)
				ShutdownMainChan <- true
				return
			} else {
				zap.S().Warnf("%s Failed to read kafka message: %s: %s", identifier, err, err.(kafka.Error).Code())
				ShutdownMainChan <- true
				return
			}
		}
		// Insert received message into the processor channel
		processorChannel <- msg
		// This is for stats only, it counts the number of messages received
		// Defined in main.go
		KafkaMessages += 1
	}
	zap.S().Debugf("%s Shutting down Kafka consumer for topic %s", identifier, topic)
}

// KafkaStartPutbackProcessor starts the putback processor.
// It will put unprocessable messages back into the kafka queue, modifying there key to include the reason and error.
func KafkaStartPutbackProcessor(identifier string, putBackChannel chan PutBackChanMsg, kafkaProducer *kafka.Producer, commitChan chan *kafka.Message) {
	// Loops until the shutdown signal is received and the channel is empty
	for !KafkaShutdownPutback {
		select {
		case msgX := <-putBackChannel:
			{
				current := time.Now().UnixMilli()
				var msg = msgX.Msg
				var reason = msgX.Reason
				var errorString = msgX.ErrorString

				if msg == nil {
					continue
				}

				topic := *msg.TopicPartition.Topic

				var kafkaKey KafkaKey
				if msg.Key == nil {
					kafkaKey = KafkaKey{
						&KafkaPutback{
							FirstTsMS: current,
							LastTsMS:  current,
							Amount:    1,
							Reason:    reason,
						},
					}
				} else {
					err := jsoniter.Unmarshal(msg.Key, &kafkaKey)
					if err != nil {
						kafkaKey = KafkaKey{
							&KafkaPutback{
								FirstTsMS: current,
								LastTsMS:  current,
								Amount:    1,
								Reason:    reason,
							},
						}
					} else {
						kafkaKey.Putback.LastTsMS = current
						kafkaKey.Putback.Amount += 1
						kafkaKey.Putback.Reason = reason
						if kafkaKey.Putback.Amount >= 2 && kafkaKey.Putback.LastTsMS-kafkaKey.Putback.FirstTsMS > 300000 {
							topic = fmt.Sprintf("putback-error-%s", *msg.TopicPartition.Topic)
							if commitChan != nil {
								commitChan <- msg
							}
						}
					}
				}

				if errorString != nil && *errorString != "" {
					kafkaKey.Putback.Error = *errorString
				}

				var err error
				msg.Key, err = jsoniter.Marshal(kafkaKey)
				if err != nil {
					zap.S().Errorf("%s Failed to marshal key: %v (%s)", identifier, kafkaKey, err)
					err = nil
				}

				msgx := kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic:     &topic,
						Partition: msg.TopicPartition.Partition,
					},
					Value: msg.Value,
					Key:   msg.Key,
				}

				err = kafkaProducer.Produce(&msgx, nil)
				if err != nil {
					putBackChannel <- PutBackChanMsg{&msgx, reason, errorString}
				}
				// This is for stats only and counts the amount of messages put back
				// Defined in main.go
				KafkaPutBacks += 1
			}
		}
	}
	zap.S().Infof("%s KafkaPutback processor shutting down", identifier)
}

// DrainChannel empties a channel into the high Throughput putback channel
func DrainChannel(identifier string, channelToDrain chan *kafka.Message, channelToDrainTo chan PutBackChanMsg) bool {
	select {
	case msg, ok := <-channelToDrain:
		if ok {
			channelToDrainTo <- PutBackChanMsg{msg, "Shutting down", nil}
		} else {
			return false
		}
	default:
		{
			return true
		}
	}
	return false
}

// KafkaStartCommitProcessor starts the commit processor.
// It will commit messages to the kafka queue.
func KafkaStartCommitProcessor(identifier string, commitChannel chan *kafka.Message, kafkaConsumer *kafka.Consumer) {
	zap.S().Infof("%s Starting commit processor", identifier)
	for !KafkaShuttingDown || len(commitChannel) > 0 {
		select {
		case msg := <-commitChannel:
			{
				_, err := kafkaConsumer.StoreMessage(msg)
				if err != nil {
					zap.S().Errorf("%s Error commiting %v: %s", identifier, msg, err)
					commitChannel <- msg
				} else {
					// This is for stats only, and counts the amounts of commits done to the kafka queue
					// Defined in main.go
					KafkaCommits += 1
				}
			}
		}
	}
	zap.S().Debugf("%s Stopped commit processor", identifier)
}

func StartEventHandler(identifier string, events chan kafka.Event, backChan chan PutBackChanMsg) {
	for !KafkaShuttingDown || len(events) > 0 {
		select {
		case event := <-events:
			switch ev := event.(type) {
			case *kafka.Message:
				{
					if ev.TopicPartition.Error != nil {
						zap.S().Errorf("Error for %s: %v", identifier, ev.TopicPartition.Error)
						errS := ev.TopicPartition.Error.Error()
						backChan <- PutBackChanMsg{
							Msg:         ev,
							Reason:      "Event channel error",
							ErrorString: &errS,
						}
					} else {
						// This is for stats only, and counts the amount of confirmed processed messages
						// Defined in main.go
						KafkaConfirmed += 1
					}
				}
			}
		default:
			time.Sleep(time.Millisecond * 100)
		}
	}
	zap.S().Debugf("%s Stopped event handler", identifier)
}
