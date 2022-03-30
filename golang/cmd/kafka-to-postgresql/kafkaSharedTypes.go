package main

import (
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
		msg, err = kafkaConsumer.ReadMessage(500)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			} else if err.(kafka.Error).Code() == kafka.ErrUnknownTopicOrPart {
				zap.S().Errorf("%s Unknown topic or partition: %s", identifier, err)
				ShutdownApplicationGraceful()
				return
			} else {
				zap.S().Warnf("%s Failed to read kafka message: %s", identifier, err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		processorChannel <- msg
		Messages += 1
	}
	zap.S().Debugf("%s Shutting down Kafka consumer for topic %s", identifier, topic)
}

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
