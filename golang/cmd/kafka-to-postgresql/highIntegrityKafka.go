package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

var highIntegrityProcessorChannel chan *kafka.Message
var highIntegrityCommitChannel chan *kafka.Message

func processHighIntegrityKafkaQueue() {
	if !HighIntegrityEnabled {
		return
	}
	zap.S().Debugf("Starting Kafka consumer for topic %s", HITopic)
	err := HIKafkaConsumer.Subscribe(HITopic, nil)
	if err != nil {
		panic(err)
	}

	for !ShuttingDown {
		if len(highIntegrityPutBackChannel) > 100 {
			// We have too many CountMessagesToCommitLater in the put back channel, so we need to wait for some to be processed
			zap.S().Debugf("Waiting for put back channel to empty: %d", len(highIntegrityPutBackChannel))
			time.Sleep(1 * time.Second)
			continue
		}

		var msg *kafka.Message
		msg, err = HIKafkaConsumer.ReadMessage(500)
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
		highIntegrityProcessorChannel <- msg
		//zap.S().Debugf("Read msg id: %d", msg.TopicPartition.Offset)
		Messages += 1
	}
	zap.S().Debugf("Shutting down Kafka consumer for topic %s", HITopic)
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

var highIntegrityPutBackChannel chan PutBackChan

func startHighIntegrityPutbackProcessor() {
	if !HighIntegrityEnabled {
		return
	}
	// Loops until the shutdown signal is received and the channel is empty
	for !ShutdownPutback {
		select {
		case msgX := <-highIntegrityPutBackChannel:
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

				msgx := kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic:     msg.TopicPartition.Topic,
						Partition: msg.TopicPartition.Partition,
					},
					Value: msg.Value,
					Key:   msg.Key,
				}

				err = HIKafkaProducer.Produce(&msgx, nil)
				if err != nil {
					highIntegrityPutBackChannel <- PutBackChan{&msgx, reason, errorString}
				}
				PutBacks += 1
			}
		}
	}
	zap.S().Infof("Putback processor shutting down")
}

// DrainHIChannel empties a channel into the high integrity putback channel
func DrainHIChannel(c chan *kafka.Message) bool {
	if !HighIntegrityEnabled {
		return true
	}
	select {
	case msg, ok := <-c:
		if ok {
			highIntegrityPutBackChannel <- PutBackChan{msg, "Shutting down", nil}
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

var HIKafkaConsumer *kafka.Consumer
var HIKafkaProducer *kafka.Producer
var HIKafkaAdminClient *kafka.AdminClient

func SetupHIKafka(configMap kafka.ConfigMap) {
	if !HighIntegrityEnabled {
		return
	}

	var err error
	HIKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	HIKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		panic(err)
	}

	HIKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	return
}

func CloseHIKafka() {
	if !HighIntegrityEnabled {
		return
	}
	zap.S().Infof("Closing Kafka Consumer")
	err := HIKafkaConsumer.Close()
	if err != nil {
		panic("Failed do close HIKafkaConsumer client !")
	}

	zap.S().Infof("Closing Kafka Producer")
	HIKafkaProducer.Flush(100)
	HIKafkaProducer.Close()

	zap.S().Infof("Closing Kafka Admin Client")
	HIKafkaAdminClient.Close()
}

func GetHICommitOffset() (lowOffset int64, highOffset int64, err error) {
	if !HighIntegrityEnabled {
		return 0, 0, nil
	}

	lowOffset, highOffset, err = HIKafkaConsumer.GetWatermarkOffsets(HITopic, 0)
	if err != nil {
		zap.S().Infof("GetWatermarkOffsets errored: %s", err)
		return 0, 0, err
	}

	return lowOffset, highOffset, err
}

func startHighIntegrityCommitProcessor() {
	zap.S().Infof("Starting HI commit processor")
	for !ShuttingDown || len(highIntegrityCommitChannel) > 0 {
		select {
		case msg := <-highIntegrityCommitChannel:
			{
				_, err := HIKafkaConsumer.StoreMessage(msg)
				//zap.S().Debugf("Stored msg id: %d", msg.TopicPartition.Offset)
				if err != nil {
					zap.S().Errorf("Error commiting %v: %s", msg, err)
					highIntegrityCommitChannel <- msg
				} else {
					Commits += 1
				}
			}
		}
	}
	zap.S().Infof("Stopped HI commit processor")
}
