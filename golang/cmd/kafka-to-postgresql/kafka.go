package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

var processorChannel chan *kafka.Message

func processKafkaQueue(topic string) {
	zap.S().Debugf("Starting Kafka consumer for topic %s", topic)
	err := internal.KafkaConsumer.Subscribe(topic, nil)
	if err != nil {
		panic(err)
	}

	for !ShuttingDown {
		if len(putBackChannel) > 100 {
			// We have too many CountMessagesToCommitLater in the put back channel, so we need to wait for some to be processed
			time.Sleep(1 * time.Second)
			continue
		}

		var msg *kafka.Message
		msg, err = internal.KafkaConsumer.ReadMessage(50)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			} else if err.(kafka.Error).Code() == kafka.ErrUnknownTopicOrPart {
				zap.S().Errorf("Unknown topic or partition: %s", err)
				ShutdownApplicationGraceful()
				return
			} else {
				zap.S().Warnf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		processorChannel <- msg
		Messages += 1
	}
	zap.S().Debugf("Shutting down Kafka consumer for topic %s", topic)
}

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

type PutBackChan struct {
	msg         *kafka.Message
	reason      string
	errorString *string
}

var putBackChannel chan PutBackChan

func startPutbackProcessor() {
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
					zap.S().Errorf("Failed to marshal key: %v (%s)", kafkaKey, err)
					err = nil
				}

				err = internal.KafkaProducer.Produce(msg, nil)
				if err != nil {
					putBackChannel <- PutBackChan{msg, reason, errorString}
				}
				PutBacks += 1
			}
		}
	}
	zap.S().Infof("Putback processor shutting down")
}

// DrainChannel empties a channel into the putback channel
func DrainChannel(c chan *kafka.Message) bool {
	select {
	case msg, ok := <-c:
		if ok {
			putBackChannel <- PutBackChan{msg, "Shutting down", nil}
		} else {
			zap.S().Debugf("Processor channel is closed !")
			return false
		}
	default:
		{
			zap.S().Debugf("Processor channel is empty !")
			return true
		}
	}
	return false
}
