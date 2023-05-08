// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"github.com/Shopify/sarama"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"runtime"
	"runtime/debug"
	"time"
)

type KafkaKey struct {
	Putback *Putback `json:"Putback,omitempty"`
}

type Putback struct {
	Reason    string `json:"Reason,omitempty"`
	Error     string `json:"Error,omitempty"`
	FirstTsMS int64  `json:"FirstTsMs"`
	LastTsMS  int64  `json:"LastTsMs"`
	Amount    int64  `json:"Amount"`
}

type PutBackChanMsg struct {
	Msg               *sarama.Message
	ErrorString       *string
	Reason            string
	ForcePutbackTopic bool
}

type PutBackProducerChanMsg struct {
	Msg               *sarama.ProducerMessage
	ErrorString       *string
	Reason            string
	ForcePutbackTopic bool
}

// KafkaCommits is a counter for the number of commits done (to the db), this is used for stats only
var KafkaCommits = float64(0)

// KafkaMessages is a counter for the number of messages processed, this is used for stats only
var KafkaMessages = float64(0)

// KafkaPutBacks is a counter for the number of messages returned to kafka, this is used for stats only
var KafkaPutBacks = float64(0)

// KafkaConfirmed is a counter for the number of messages confirmed to kafka, this is used for stats only
var KafkaConfirmed = float64(0)

var LastKafkaMessageReceived = time.Now()

var ShuttingDownKafka bool
var ShutdownPutback bool
var nearMemoryLimit = false

//goland:noinspection GoUnusedExportedFunction
func MemoryLimiter(allowedMemorySize int) {
	allowedSeventyFivePerc := uint64(float64(allowedMemorySize) * 0.9)
	allowedNintyPerc := uint64(float64(allowedMemorySize) * 0.75)
	for {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		if m.Alloc > allowedNintyPerc {
			zap.S().Errorf("Memory usage is too high: %d bytes, slowing ingress !", m.TotalAlloc)
			nearMemoryLimit = true
			debug.FreeOSMemory()
			time.Sleep(internal.FiveSeconds)
		}
		if m.Alloc > allowedSeventyFivePerc {
			zap.S().Errorf("Memory usage is high: %d bytes !", m.TotalAlloc)
			nearMemoryLimit = false
			runtime.GC()
			time.Sleep(internal.FiveSeconds)
		} else {
			nearMemoryLimit = false
			time.Sleep(internal.OneSecond)
		}
	}
}

var putBackRetries = 0

// ProcessKafkaQueue processes the kafka queue and sends the messages to the processorChannel.
// It uses topic as regex for subscribing to kafka topics.
// If the putback channel is full, it will block until the channel is free.
func ProcessKafkaQueue(
	identifier string,
	topic string,
	processorChannel chan *kafka.Message,
	kafkaConsumer *kafka.Client,
	putBackChannel chan PutBackProducerChanMsg,
	gracefulShutdown func()) {

	zap.S().Debugf("%s Starting Kafka consumer for topic %s", identifier, topic)

	for !ShuttingDownKafka {
		if len(putBackChannel) > 100 {
			// We have too many CountMessagesToCommitLater in the put back channel, so we need to wait for some to be processed
			zap.S().Debugf("%s Waiting for put back channel to empty: %d", identifier, len(putBackChannel))
			putBackRetries += 1
			time.Sleep(internal.OneSecond)
			if putBackRetries > 10 {
				zap.S().Errorf("%s Put back channel is full for too long, shutting down", identifier)
				for i := 0; i < 100; i++ {
					zap.S().Errorf("We just lost data !")
				}
				gracefulShutdown()
			}
			continue
		}

		if nearMemoryLimit {
			time.Sleep(internal.OneSecond)
			continue
		}

		msg, isShuttingDown := waitNewMessages(identifier, kafkaConsumer, gracefulShutdown)
		if msg == nil {
			if isShuttingDown {
				return
			}
			continue
		}
		LastKafkaMessageReceived = time.Now()
		// Insert received message into the processor channel
		processorChannel <- msg
		// This is for stats only, it counts the number of messages received
		KafkaMessages += 1
	}
	zap.S().Debugf("%s Shutting down Kafka consumer for topic %s", identifier, topic)
}

// ProcessKafkaTopicProbeQueue processes the kafka queue and sends the messages to the processorChannel.
// It only subscribes to the topic used to announce a new topic.
func ProcessKafkaTopicProbeQueue(identifier string, processorChannel chan *kafka.Message, gracefulShutdown func()) {
	for !ShuttingDownKafka {
		msg, isShuttingDown := waitNewMessages(identifier, KafkaTopicProbeConsumer, gracefulShutdown)
		if msg == nil {
			if isShuttingDown {
				return
			}
			continue
		}
		// Insert received message into the processor channel
		processorChannel <- msg
		// This is for stats only, it counts the number of messages received
		KafkaMessages += 1
	}
	zap.S().Debugf("%s Shutting down Kafka Topic Probe consumer", identifier)
}

// waitNewMessages waits for new messages on the kafka consumer and checks for errors
func waitNewMessages(identifier string, kafkaConsumer *kafka.Client, gracefulShutdown func()) (
	msg *kafka.Message,
	isShuttingDown bool) {
	// Wait for new messages. A Sarama Kafka Consumer gets timeout length at initialization.
	var errConsumer <-chan error = kafkaConsumer.Consumer.Errors()
	var err error

	select {
	case err = <-errConsumer:
		zap.S().Warnf("%s Failed to read kafka message: %s", identifier, err)
		gracefulShutdown()
	}
	return msg, false
}

// StartPutbackProcessor starts the putback processor.
// It will put unprocessable messages back into the kafka queue, modifying there key to include the Reason and error.
func StartPutbackProcessor(
	identifier string,
	putBackChannel chan PutBackProducerChanMsg,
	kafkaProducer *kafka.Client,
	commitChannel chan *sarama.ProducerMessage,
	putbackChanSize int) {
	zap.S().Debugf("%s Starting putback processor", identifier)
	// Loops until the shutdown signal is received and the channel is empty
	for !ShutdownPutback {
		msgX := <-putBackChannel

		current := time.Now().UnixMilli()
		var msg = msgX.Msg
		var reason = msgX.Reason
		var errorString = msgX.ErrorString

		if msg == nil {
			continue
		}

		var msg2 sarama.ProducerMessage
		if msg.Value != nil {
			msg2.Value = msg.Value
		}

		msg2.Partition = 0

		topic := msg.Topic

		var rawKafkaKey []byte
		var putbackIndex = -1

		// Check for new header based putback info
		msg2.Headers = msg.Headers
		for i, header := range msg2.Headers {
			if string(header.Key) == "putback" {
				rawKafkaKey = header.Value
				putbackIndex = i
				break
			}
		}

		var kafkaKey KafkaKey

		if rawKafkaKey == nil {
			kafkaKey = KafkaKey{
				&Putback{
					FirstTsMS: current,
					LastTsMS:  current,
					Amount:    1,
					Reason:    reason,
				},
			}
		} else {
			err := jsoniter.Unmarshal(rawKafkaKey, &kafkaKey)
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
				if msgX.ForcePutbackTopic || (kafkaKey.Putback.Amount >= 2 && kafkaKey.Putback.LastTsMS-kafkaKey.Putback.FirstTsMS > 300000) {
					topic = fmt.Sprintf("putback-error-%s", msg.Topic)

					if commitChannel != nil {
						commitChannel <- msg
					}
				}
			}
		}

		if errorString != nil && *errorString != "" {
			kafkaKey.Putback.Error = *errorString
		}

		var err error
		var header []byte
		header, err = jsoniter.Marshal(kafkaKey)
		if err != nil {
			zap.S().Errorf("%s Failed to marshal key: %v (%s)", identifier, kafkaKey, err)
		}
		if putbackIndex == -1 {
			msg2.Headers = append(
				msg.Headers, sarama.RecordHeader{
					Key:   []byte("putback"),
					Value: header,
				})
		} else {
			msg2.Headers[putbackIndex] = sarama.RecordHeader{
				Key:   []byte("putback"),
				Value: header,
			}
		}

		putBackValue, err := msg2.Value.Encode()

		if err != nil {
			zap.S().Errorf("%s Failed to encode putback value: %s", identifier, err)
		}

		var putbackHeaders map[string][]byte

		for _, v := range msg2.Headers {
			putbackHeaders[string(v.Key)] = v.Value
		}

		msgx := kafka.Message{
			Topic:  topic,
			Value:  putBackValue,
			Header: putbackHeaders,
		}
		err = kafkaProducer.EnqueueMessage(msgx)
		if err != nil {
			zap.S().Warnf("%s Failed to produce putback message: %s", identifier, err)
			if len(putBackChannel) >= putbackChanSize {
				// This don't have to be graceful, as putback is already broken
				panic(fmt.Errorf("putback channel full, shutting down"))
			}
			prodMsg := sarama.ProducerMessage{Topic: topic, Value: msg2.Value, Headers: msg2.Headers}
			putBackChannel <- PutBackProducerChanMsg{&prodMsg, errorString, reason, false}
		}
		// This is for stats only and counts the amount of messages put back
		KafkaPutBacks += 1
		// Commit original message, after putback duplicate has been produced !
		commitChannel <- msg

	}
	zap.S().Infof("%s Putback processor shutting down", identifier)
}

// DrainChannel empties a channel into the high Throughput putback channel
func DrainChannel(
	identifier string,
	channelToDrain chan *kafka.Message,
	channelToDrainTo chan PutBackProducerChanMsg,
	ShutdownChannel chan bool) bool {
	for len(channelToDrain) > 0 {
		select {
		case msg, ok := <-channelToDrain:
			if ok {
				channelToDrainTo <- PutBackProducerChanMsg{kafka.MessageToProducerMessage(msg), nil, fmt.Sprintf("%s Shutting down", identifier), false}
				KafkaPutBacks += 1
			} else {
				zap.S().Warnf("%s Channel to drain is closed", identifier)
				if ShutdownChannel != nil {
					ShutdownChannel <- false
				}
				return false
			}
		default:
			{
				zap.S().Debugf("%s Channel to drain is empty", identifier)
				if ShutdownChannel != nil {
					ShutdownChannel <- true
				}
				return true
			}
		}
	}
	zap.S().Debugf("%s channel drained", identifier)
	if ShutdownChannel != nil {
		ShutdownChannel <- true
	}
	return true
}

// DrainChannelSimple empties a channel into another channel
func DrainChannelSimple(channelToDrain chan *kafka.Message, channelToDrainTo chan PutBackProducerChanMsg) bool {
	select {
	case msg, ok := <-channelToDrain:
		if ok {
			channelToDrainTo <- PutBackProducerChanMsg{kafka.MessageToProducerMessage(msg), nil, "Shutting down", false}
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

// StartCommitProcessor starts the commit processor.
// It will commit messages to the kafka queue.
func StartCommitProcessor(identifier string, commitChannel chan *sarama.ProducerMessage, kafkaConsumer *kafka.Client) {
	zap.S().Debugf("%s Starting commit processor", identifier)
	for !ShuttingDownKafka || len(commitChannel) > 0 {
		// TODO: StoreOffset. How can I store messages (offsets) by sarama Kafka?
		/*msg := <-commitChannel
		_, err := kafkaConsumer.GetMessages()
		if err != nil {
			zap.S().Errorf("%s Error committing %v: %s", identifier, msg, err)
			commitChannel <- msg
		} else {
			// This is for stats only, and counts the amounts of commits done to the kafka queue

			KafkaCommits += 1
		}*/

	}
	zap.S().Debugf("%s Stopped commit processor", identifier)
}

func StartProducerEventHandler(identifier string, errors <-chan *sarama.ProducerError, successes <-chan *sarama.ProducerMessage, backChan chan PutBackProducerChanMsg) {
	zap.S().Debugf("%s Starting event handler", identifier)
	var errorProducer sarama.ProducerError
	for !ShuttingDownKafka || len(errors) > 0 || len(successes) > 0 {
		select {
		case errorProducer = <-errors:
			errS := errorProducer.Error()
			zap.S().Errorf("Error for %s: %s", identifier, errS)
			if backChan != nil {
				backChan <- PutBackProducerChanMsg{
					Msg:         errorProducer.Msg,
					Reason:      "ProducerEvent channel error",
					ErrorString: &errS,
				}
			}
		case _ = <-successes:
			KafkaConfirmed += 1
		default:
			time.Sleep(time.Millisecond * 100)
		}
	}
	zap.S().Debugf("%s Stopped event handler", identifier)
}

func StartConsumerEventHandler(identifier string, errors <-chan error, msgs <-chan kafka.Message) {
	zap.S().Debugf("%s Starting event handler", identifier)
	var errorConsumer sarama.ConsumerError
	for !ShuttingDownKafka || len(errors) > 0 {
		select {
		case errorConsumer = <-errors:
			errS := errorConsumer.Error()
			zap.S().Errorf("Error for %s: %s", identifier, errS)
		case _ = <-msgs:
			KafkaConfirmed += 1
		default:
			time.Sleep(time.Millisecond * 100)
		}
	}
	zap.S().Debugf("%s Stopped event handler", identifier)
}
