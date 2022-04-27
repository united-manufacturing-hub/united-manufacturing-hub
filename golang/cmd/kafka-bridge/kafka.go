package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

type KafkaKey struct {
	Putback *Putback `json:"Putback,omitempty"`
}

type Putback struct {
	FirstTsMS int64  `json:"FirstTsMs"`
	LastTsMS  int64  `json:"LastTsMs"`
	Amount    int64  `json:"Amount"`
	Reason    string `json:"Reason,omitempty"`
	Error     string `json:"Error,omitempty"`
}

type PutBackChanMsg struct {
	msg         *kafka.Message
	reason      string
	errorString *string
}

// processKafkaQueue processes the kafka queue and sends the messages to the processorChannel.
// It uses topic as regex for subscribing to kafka topics.
// If the putback channel is full, it will block until the channel is free.
func processKafkaQueue(identifier string, topic string, processorChannel chan *kafka.Message, kafkaConsumer *kafka.Consumer, putBackChannel chan PutBackChanMsg) {
	zap.S().Debugf("%s Starting Kafka consumer for topic %s", identifier, topic)
	err := kafkaConsumer.Subscribe(topic, nil)
	if err != nil {
		panic(err)
	}

	for !ShuttingDown {
		if len(putBackChannel) > 100 {
			// We have too many CountMessagesToCommitLater in the put back channel, so we need to wait for some to be processed
			zap.S().Debugf("%s Waiting for put back channel to empty: %d", identifier, len(putBackChannel))
			time.Sleep(1 * time.Second)
			continue
		}

		var msg *kafka.Message
		// Wait for new messages
		// This has a timeout, allowing ShuttingDown to be checked
		msg, err = kafkaConsumer.ReadMessage(5000)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			} else if err.(kafka.Error).Code() == kafka.ErrUnknownTopicOrPart {
				// This will occur when no topic for the regex is available !
				zap.S().Errorf("%s Unknown topic or partition: %s", identifier, err)
				ShutdownApplicationGraceful()
				return
			} else {
				zap.S().Warnf("%s Failed to read kafka message: %s: %s", identifier, err, err.(kafka.Error).Code())
				ShutdownApplicationGraceful()
				return
			}
		}
		// Insert received message into the processor channel
		processorChannel <- msg
		// Increase message counter, for stats
		Messages += 1
	}
	zap.S().Debugf("%s Shutting down Kafka consumer for topic %s", identifier, topic)
}

// startPutbackProcessor starts the putback processor.
// It will put unprocessable messages back into the kafka queue, modifying there key to include the reason and error.
func startPutbackProcessor(identifier string, putBackChannel chan PutBackChanMsg, kafkaProducer *kafka.Producer) {
	// Loops until the shutdown signal is received and the channel is empty
	for !ShutdownPutback {
		select {
		case msgX := <-putBackChannel:
			{
				current := time.Now().UnixMilli()
				var msg = msgX.msg
				var reason = msgX.reason
				var errorString = msgX.errorString

				if msg == nil {
					continue
				}

				var kafkaKey KafkaKey
				if msg.Key == nil {
					kafkaKey = KafkaKey{
						&Putback{
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
							&Putback{
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
						Topic:     msg.TopicPartition.Topic,
						Partition: msg.TopicPartition.Partition,
					},
					Value: msg.Value,
					Key:   msg.Key,
				}

				err = kafkaProducer.Produce(&msgx, nil)
				if err != nil {
					putBackChannel <- PutBackChanMsg{&msgx, reason, errorString}
				}
				PutBacks += 1
			}
		}
	}
	zap.S().Infof("%s Putback processor shutting down", identifier)
}

// DrainChannel empties a channel into the high Throughput putback channel
func DrainChannel(identifier string, channelToDrain chan *kafka.Message, channelToDrainTo chan PutBackChanMsg) bool {
	for len(channelToDrain) > 0 {
		select {
		case msg, ok := <-channelToDrain:
			if ok {
				channelToDrainTo <- PutBackChanMsg{msg, fmt.Sprintf("%s Shutting down", identifier), nil}
				PutBacks += 1
			} else {
				zap.S().Warnf("%s Channel to drain is closed", identifier)
				ShutdownChannel <- false
				return false
			}
		default:
			{
				zap.S().Debugf("%s Channel to drain is empty", identifier)
				ShutdownChannel <- true
				return true
			}
		}
	}
	zap.S().Debugf("%s channel drained", identifier)
	ShutdownChannel <- true
	return true
}

// startCommitProcessor starts the commit processor.
// It will commit messages to the kafka queue.
func startCommitProcessor(identifier string, commitChannel chan *kafka.Message, kafkaConsumer *kafka.Consumer) {
	zap.S().Infof("%s Starting commit processor", identifier)
	for !ShuttingDown || len(commitChannel) > 0 {
		select {
		case msg := <-commitChannel:
			{
				_, err := kafkaConsumer.StoreMessage(msg)
				if err != nil {
					zap.S().Errorf("%s Error commiting %v: %s", identifier, msg, err)
					commitChannel <- msg
				} else {
					Commits += 1
				}
			}
		}
	}
	zap.S().Infof("%s Stopped commit processor", identifier)
}
