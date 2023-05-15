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

package processor

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-bridge/message"
	"go.uber.org/zap"
	"regexp"
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
	Msg               *kafka.Message
	ErrorString       *string
	Reason            string
	ForcePutbackTopic bool
}

// client1 is the Kafka client that gets messages from the broker
var client1 *kafka.Client

// client2 is the Kafka client that sends messages to the broker
var client2 *kafka.Client
var ShutdownPutback bool
var KafkaPutBacks = float64(0)

// CreateClient creates the kafka clients and starts the message processing loop.
//
// listenTopicRegex is the regex that the clients will use to subscribe to topics.
// client1Options are the options for the client that gets messages from the broker.
// client2Options are the options for the client that sends messages to the broker.
// kafkaChan is the channel that the messages will be sent to.
func CreateClient(listenTopicRegex *regexp.Regexp, client1Options, client2Options kafka.NewClientOptions, kafkaChan chan kafka.Message) {
	client1Options.ListenTopicRegex = listenTopicRegex
	client2Options.ListenTopicRegex = listenTopicRegex

	var err error
	client1, err = kafka.NewKafkaClient(client1Options)
	if err != nil {
		zap.S().Fatalf("Error creating local kafka client: %v", err)
	}
	client2, err = kafka.NewKafkaClient(client2Options)
	if err != nil {
		zap.S().Fatalf("Error creating remote kafka client: %v", err)
	}

	go processIncomingMessage(kafkaChan)
}

// processIncomingMessage gets messages from the client that gets messages from the broker and sends them to the kafkaChan.
func processIncomingMessage(kafkaChan chan kafka.Message) {
	for {
		msg := <-client1.GetMessages()
		kafkaChan <- msg
	}
}

// Start starts enqueuing messages from client1 to client2.
func Start(kafkaChan chan kafka.Message) {
	for {
		msg := <-kafkaChan
		if !message.IsValidKafkaMessage(msg) {
			continue
		}
		err := client2.EnqueueMessage(msg)
		if err != nil {
			zap.S().Errorf("Error sending message to kafka: %v", err)
		}
	}
}

// Shutdown shuts down the kafka clients.
func Shutdown() {
	zap.S().Info("Shutting down kafka clients")
	err := client1.Close()
	if err != nil {
		zap.S().Errorf("Error closing kafka client: %v", err)
	}
	err = client2.Close()
	if err != nil {
		zap.S().Errorf("Error closing kafka client: %v", err)
	}
	zap.S().Info("Kafka clients shut down")
}

// GetStats gets the stats from the kafka clients.
func GetStats() (sent uint64, received uint64) {
	return kafka.GetKafkaStats()
}

// StartPutbackProcessor starts the putback processor.
// It will put unprocessable messages back into the kafka queue, modifying there key to include the Reason and error.
func StartPutbackProcessor(identifier string, putBackChannel chan PutBackChanMsg, commitChannel chan kafka.Message) {
	zap.S().Debugf("%s Starting putback processor", identifier)
	// Loops until the shutdown signal is received and the channel is empty
	for !ShutdownPutback {
		putBackChanMsg := <-putBackChannel

		current := time.Now().UnixMilli()
		var originalMsg = putBackChanMsg.Msg
		var reason = putBackChanMsg.Reason
		var errorString = putBackChanMsg.ErrorString

		if originalMsg == nil {
			continue
		}

		topic := originalMsg.Topic

		newMsg := kafka.Message{
			Topic:  topic,
			Header: originalMsg.Header,
		}
		if originalMsg.Value != nil {
			newMsg.Value = originalMsg.Value
		}

		var rawKafkaKey []byte
		var putbackKey = ""

		// Check for putback header
		for key, header := range originalMsg.Header {
			if key == "putback" {
				rawKafkaKey = header
				putbackKey = key
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
				if putBackChanMsg.ForcePutbackTopic || (kafkaKey.Putback.Amount >= 2 && kafkaKey.Putback.LastTsMS-kafkaKey.Putback.FirstTsMS > 300000) {
					topic = fmt.Sprintf("putback-error-%s", originalMsg.Topic)

					if commitChannel != nil {
						commitChannel <- *originalMsg
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
		if putbackKey == "" {
			newMsg.Header["putback"] = header
		} else {
			newMsg.Header[putbackKey] = header
		}

		err = client2.EnqueueMessage(newMsg)
		if err != nil {
			zap.S().Warnf("%s Failed to produce putback message: %s", identifier, err)
			if len(putBackChannel) >= cap(putBackChannel) {
				// This don't have to be graceful, as putback is already broken
				panic(fmt.Errorf("putback channel full, shutting down"))
			}
			putBackChannel <- PutBackChanMsg{&newMsg, errorString, reason, false}
		}
		// This is for stats only and counts the amount of messages put back
		KafkaPutBacks += 1
		// Commit original message, after putback duplicate has been produced !
		commitChannel <- *originalMsg

	}
	zap.S().Infof("%s Putback processor shutting down", identifier)
}
