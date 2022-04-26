package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
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
		zap.S().Errorf("%s Failed to subscribe to topic %s: %s", identifier, topic, err)
		panic(err)
	}

	for !ShuttingDown {
		if len(putBackChannel) > 100 {
			// We have too many CountMessagesToCommitLater in the put back channel, so we need to wait for some to be processed
			zap.S().Debugf("%s Waiting for put back channel to empty: %d", identifier, len(putBackChannel))
			time.Sleep(internal.OneSecond)
			continue
		}

		if nearMemoryLimit {
			time.Sleep(internal.OneSecond)
			continue
		}

		var msg *kafka.Message
		// Wait for new messages
		// This has a timeout, allowing ShuttingDown to be checked
		msg, err = kafkaConsumer.ReadMessage(5000)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				// This is fine, and expected behaviour
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
		// This is for stats only, it counts the number of messages received
		// Defined in main.go
		Messages += 1
	}
	zap.S().Debugf("%s Shutting down Kafka consumer for topic %s", identifier, topic)
}

// startPutbackProcessor starts the putback processor.
// It will put unprocessable messages back into the kafka queue, modifying there key to include the reason and error.
func startPutbackProcessor(identifier string, putBackChannel chan PutBackChanMsg, kafkaProducer *kafka.Producer) {
	zap.S().Debugf("%s Starting putback processor", identifier)
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

				topic := *msg.TopicPartition.Topic

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
						if kafkaKey.Putback.Amount >= 50 {
							topic = "putback-error"
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

// startCommitProcessor starts the commit processor.
// It will commit messages to the kafka queue.
func startCommitProcessor(identifier string, commitChannel chan *kafka.Message, kafkaConsumer *kafka.Consumer) {
	zap.S().Debugf("%s Starting commit processor", identifier)
	for !ShuttingDown || len(commitChannel) > 0 {
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
					Commits += 1
				}
			}
		}
	}
	zap.S().Debugf("%s Stopped commit processor", identifier)
}

func startEventHandler(identifier string, events chan kafka.Event, backChan chan PutBackChanMsg) {
	zap.S().Debugf("%s Starting event handler", identifier)
	for !ShuttingDown || len(events) > 0 {
		select {
		case event := <-events:
			switch ev := event.(type) {
			case *kafka.Message:
				{
					if ev.TopicPartition.Error != nil {
						zap.S().Errorf("Error for %s: %v", identifier, ev.TopicPartition.Error)
						errS := ev.TopicPartition.Error.Error()
						backChan <- PutBackChanMsg{
							msg:         ev,
							reason:      "Event channel error",
							errorString: &errS,
						}
					} else {
						// This is for stats only, and counts the amount of confirmed processed messages
						// Defined in main.go
						Confirmed += 1
					}
				}
			}
		default:
			time.Sleep(time.Millisecond * 100)
		}
	}
	zap.S().Debugf("%s Stopped event handler", identifier)
}
