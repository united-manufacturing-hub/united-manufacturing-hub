package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

var ActivityProcessorChannel chan *kafka.Message
var ActivityCommitChannel chan *kafka.Message
var ActivityPutBackChannel chan internal.PutBackChanMsg

// ActivityKafkaConsumer is a high Integrity Kafka consumer
var ActivityKafkaConsumer *kafka.Consumer

// ActivityKafkaProducer is a high Integrity Kafka producer
var ActivityKafkaProducer *kafka.Producer

// ActivityKafkaAdminClient is a high Integrity Kafka admin
var ActivityKafkaAdminClient *kafka.AdminClient

// SetupActivityKafka sets up the Activity Kafka consumer, producer and admin
func SetupActivityKafka(configMap kafka.ConfigMap) {
	if !ActivityEnabled {
		return
	}

	var err error
	ActivityKafkaConsumer, err = kafka.NewConsumer(&configMap)
	if err != nil {
		panic(err)
	}

	ActivityKafkaProducer, err = kafka.NewProducer(&configMap)
	if err != nil {
		panic(err)
	}

	ActivityKafkaAdminClient, err = kafka.NewAdminClient(&configMap)
	if err != nil {
		panic(err)
	}

	return
}

// CloseActivityKafka closes the Activity Kafka consumer, producer and admin
func CloseActivityKafka() {
	if !ActivityEnabled {
		return
	}
	zap.S().Infof("[Activity]Closing Kafka Consumer")
	err := ActivityKafkaConsumer.Close()
	if err != nil {
		panic("Failed do close ActivityKafkaConsumer client !")
	}

	zap.S().Infof("[Activity]Closing Kafka Producer")
	ActivityKafkaProducer.Flush(100)
	ActivityKafkaProducer.Close()

	zap.S().Infof("[Activity]Closing Kafka Admin Client")
	ActivityKafkaAdminClient.Close()
}

func startActivityProcessor() {
	for !ShuttingDown {
		var msg *kafka.Message
		// Get next message from HI kafka consumer
		msg = <-activityProcessorChannel
		if msg == nil {
			continue
		}
		parsed, parsedMessage := internal.ParseMessage(msg)
		if !parsed {
			continue
		}

		if parsedMessage.PayloadType != "activity" {
			continue
		}

		var activityMessage datamodel.Activity
		err := jsoniter.Unmarshal(parsedMessage.Payload, &activityMessage)
		if err != nil {
			zap.S().Errorf("[AC] Failed to parse activity message: %s", err)
			continue
		}

		var stateMessage datamodel.State
		if activityMessage.Activity {
			stateMessage.State = datamodel.ProducingAtFullSpeedState
		} else {
			stateMessage.State = datamodel.UnspecifiedStopState
		}

		jsonStateMessage, err := jsoniter.Marshal(stateMessage)
		if err != nil {
			continue
		}

		stateTopic := fmt.Sprintf("ia.%s.%s.%s.state", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId)
		var msgS *kafka.Message
		msgS = &kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &stateTopic, Partition: kafka.PartitionAny},
			Value:          jsonStateMessage,
		}
		err = ActivityKafkaProducer.Produce(msgS, nil)
		if err != nil {
			errS := err.Error()
			ActivityPutBackChannel <- internal.PutBackChanMsg{
				Msg:         msg,
				Reason:      "Failed to produce state message",
				ErrorString: &errS,
			}
			continue
		}
		activityCommitChannel <- msg
	}
}
