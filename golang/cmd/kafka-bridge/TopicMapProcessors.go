package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/coocood/freecache"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"os"
	"time"
)

var messageCache *freecache.Cache

// CreateTopicMapProcessors creates a new TopicMapProcessor for each topic in the map.
// It also initialized the message cache, which prevents duplicate messages from being sent to the Kafka broker and circular messages from being processed.
func CreateTopicMapProcessors(tp TopicMap, kafkaGroupIdSuffic string, securityProtocol string) {
	// 1Gb cache
	messageCache = freecache.NewCache(1024 * 1024 * 1024)

	localConfigMap := kafka.ConfigMap{
		"bootstrap.servers":        LocalKafkaBootstrapServers,
		"security.protocol":        securityProtocol,
		"ssl.key.location":         "/SSL_certs/tls.key",
		"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
		"ssl.certificate.location": "/SSL_certs/tls.crt",
		"ssl.ca.location":          "/SSL_certs/ca.crt",
		"group.id":                 fmt.Sprintf("kafka-bridge-local-%s", kafkaGroupIdSuffic),
		"auto.offset.reset":        "earliest",
		"enable.auto.commit":       true,
		"enable.auto.offset.store": false,
	}

	remoteConfigMap := kafka.ConfigMap{
		"bootstrap.servers":        RemoteKafkaBootstrapServers,
		"security.protocol":        securityProtocol,
		"ssl.key.location":         "/SSL_certs/tls.key",
		"ssl.key.password":         os.Getenv("KAFKA_SSL_KEY_PASSWORD"),
		"ssl.certificate.location": "/SSL_certs/tls.crt",
		"ssl.ca.location":          "/SSL_certs/ca.crt",
		"group.id":                 fmt.Sprintf("kafka-bridge-remote-%s", kafkaGroupIdSuffic),
		"auto.offset.reset":        "earliest",
		"enable.auto.commit":       true,
		"enable.auto.offset.store": false,
	}
	for _, element := range tp {
		go CreateTopicMapElementProcessor(element, localConfigMap, remoteConfigMap)
	}
}

// CreateTopicMapElementProcessor create uni/bidirectional transfer channels between the local and remote Kafka brokers.
func CreateTopicMapElementProcessor(element TopicMapElement, localConfigMap kafka.ConfigMap, remoteConfigMap kafka.ConfigMap) {
	zap.S().Debugf("Creating TopicMapProcessor for topic %v", element)
	zap.S().Infof("Starting Processor with local configmap: %v", localConfigMap)
	zap.S().Infof("Starting Processor with remote configmap: %v", remoteConfigMap)

	if element.Bidirectional {
		var localConsumer, err = kafka.NewConsumer(&localConfigMap)
		if err != nil {
			panic(fmt.Sprintf("Failed to create localConsumer: %v for element %v", err, element))
		}

		var localProducer *kafka.Producer
		localProducer, err = kafka.NewProducer(&localConfigMap)
		if err != nil {
			panic(fmt.Sprintf("Failed to create localProducer: %v for element %v", err, element))
		}

		var remoteConsumer *kafka.Consumer
		remoteConsumer, err = kafka.NewConsumer(&remoteConfigMap)
		if err != nil {
			panic(fmt.Sprintf("Failed to create localConsumer: %v for element %v", err, element))
		}

		var remoteProducer *kafka.Producer
		remoteProducer, err = kafka.NewProducer(&remoteConfigMap)
		if err != nil {
			panic(fmt.Sprintf("Failed to create localProducer: %v for element %v", err, element))
		}

		localMsgChan := make(chan *kafka.Message, 100)
		localPutBackChan := make(chan internal.PutBackChanMsg, 100)
		localCommitChan := make(chan *kafka.Message, 100)
		localIdentifier := fmt.Sprintf("%s-local-%s", element.Name, os.Getenv("SERIAL_NUMBER"))
		go internal.ProcessKafkaQueue(localIdentifier, element.Topic, localMsgChan, localConsumer, localPutBackChan, nil)
		go internal.StartPutbackProcessor(localIdentifier, localPutBackChan, localProducer, localCommitChan, 100)
		go internal.StartCommitProcessor(localIdentifier, localCommitChan, localConsumer)

		remoteMsgChan := make(chan *kafka.Message, 100)
		remotePutBackChan := make(chan internal.PutBackChanMsg, 100)
		remoteCommitChan := make(chan *kafka.Message, 100)
		remoteIdentifier := fmt.Sprintf("%s-remote-%s", element.Name, os.Getenv("SERIAL_NUMBER"))
		go internal.ProcessKafkaQueue(remoteIdentifier, element.Topic, remoteMsgChan, remoteConsumer, remotePutBackChan, nil)
		go internal.StartPutbackProcessor(remoteIdentifier, remotePutBackChan, remoteProducer, remoteCommitChan, 100)
		go internal.StartCommitProcessor(remoteIdentifier, remoteCommitChan, remoteConsumer)

		go startAtoBSender(localIdentifier, localMsgChan, remoteProducer, localPutBackChan, localCommitChan)
		go startAtoBSender(remoteIdentifier, remoteMsgChan, localProducer, remotePutBackChan, remoteCommitChan)

		ShutdownsRequired += 2
		for !ShuttingDown {
			time.Sleep(internal.OneSecond)
		}
		internal.DrainChannel(localIdentifier, localMsgChan, localPutBackChan, ShutdownChannel)
		internal.DrainChannel(remoteIdentifier, remoteMsgChan, remotePutBackChan, ShutdownChannel)
	} else {
		var consumer *kafka.Consumer
		var producer *kafka.Producer
		var putBackProducer *kafka.Producer
		var err error

		if element.SendDirection == ToLocal {
			consumer, err = kafka.NewConsumer(&remoteConfigMap)
			if err != nil {
				panic(fmt.Sprintf("Failed to create consumer: %v for element %v", err, element))
			}
			putBackProducer, err = kafka.NewProducer(&remoteConfigMap)
			if err != nil {
				panic(fmt.Sprintf("Failed to create producer: %v for element %v", err, element))
			}
			producer, err = kafka.NewProducer(&localConfigMap)
			if err != nil {
				panic(fmt.Sprintf("Failed to create producer: %v for element %v", err, element))
			}

		} else if element.SendDirection == ToRemote {
			consumer, err = kafka.NewConsumer(&localConfigMap)
			if err != nil {
				panic(fmt.Sprintf("Failed to create consumer: %v for element %v", err, element))
			}
			putBackProducer, err = kafka.NewProducer(&localConfigMap)
			if err != nil {
				panic(fmt.Sprintf("Failed to create producer: %v for element %v", err, element))
			}
			producer, err = kafka.NewProducer(&remoteConfigMap)
			if err != nil {
				panic(fmt.Sprintf("Failed to create producer: %v for element %v", err, element))
			}
		} else {
			panic(fmt.Sprintf("Invalid send direction %v for element %v", element.SendDirection, element))
		}

		msgChan := make(chan *kafka.Message, 100)
		putBackChan := make(chan internal.PutBackChanMsg, 100)
		commitChan := make(chan *kafka.Message, 100)
		identifier := fmt.Sprintf("%s-local", element.Name)
		go internal.ProcessKafkaQueue(identifier, element.Topic, msgChan, consumer, putBackChan, nil)
		go internal.StartPutbackProcessor(identifier, putBackChan, putBackProducer, commitChan, 100)
		go internal.StartCommitProcessor(identifier, commitChan, consumer)
		go startAtoBSender(identifier, msgChan, producer, putBackChan, commitChan)
		go internal.StartEventHandler(identifier, producer.Events(), putBackChan)

		ShutdownsRequired += 1
		for !ShuttingDown {
			time.Sleep(internal.OneSecond)
		}
		internal.DrainChannel(identifier, msgChan, putBackChan, ShutdownChannel)
	}
}

func MessageAlreadyTransmitted(msg *kafka.Message) bool {
	xxhasher := xxh3.New()
	_, _ = xxhasher.Write(msg.Value)
	if msg.TopicPartition.Topic != nil {
		_, _ = xxhasher.WriteString(*msg.TopicPartition.Topic)
	}
	buf := new(bytes.Buffer)
	// Can never fail
	_ = binary.Write(buf, binary.LittleEndian, msg.TopicPartition.Partition)
	_, _ = xxhasher.Write(buf.Bytes())

	key := xxhasher.Sum128().Bytes()

	getOrSet, _ := messageCache.GetOrSet(key[:], []byte{1}, 0)

	return getOrSet != nil

}

type Bridges []string

func UnmarshalBridges(data []byte) (Bridges, error) {
	var r Bridges
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *Bridges) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

func startAtoBSender(identifier string, msgChan chan *kafka.Message, producer *kafka.Producer, backChan chan internal.PutBackChanMsg, commitChan chan *kafka.Message) {

	for !ShuttingDown {
		msg := <-msgChan

		if MessageAlreadyTransmitted(msg) {
			commitChan <- msg
			continue
		}

		var foundPreviousBridges bool
		for i, header := range msg.Headers {
			if header.Key == "x-bridges" {
				bridges, err := UnmarshalBridges(header.Value)
				if err == nil {
					bridges = append(bridges, identifier)
					var val []byte
					val, err = bridges.Marshal()
					if err == nil {
						msg.Headers[i].Value = val
						foundPreviousBridges = true
					}
				}
				break
			}
		}
		if !foundPreviousBridges {
			bridges := Bridges{identifier}
			var val []byte
			val, err := bridges.Marshal()
			if err == nil {
				msg.Headers = append(msg.Headers, kafka.Header{
					Key:   "x-bridges",
					Value: val,
				})
			}
		}

		msg.Headers = append(msg.Headers, kafka.Header{
			Key:   "x-last-bridge-id",
			Value: []byte(identifier),
		})
		msg.Headers = append(msg.Headers, kafka.Header{
			Key:   "x-last-bridge-time-ms",
			Value: []byte(fmt.Sprintf("%d", time.Now().UnixMilli())),
		})

		msgX := kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     msg.TopicPartition.Topic,
				Partition: kafka.PartitionAny,
			},
			Value:   msg.Value,
			Key:     msg.Key,
			Headers: msg.Headers,
		}

		err := producer.Produce(&msgX, nil)
		if err != nil {
			errS := err.Error()
			backChan <- internal.PutBackChanMsg{
				Msg:         &msgX,
				Reason:      "Produce failed",
				ErrorString: &errS,
			}
			zap.S().Warnf("[%s] Failed to produce message: %v | %#v", identifier, err, msgX)
			time.Sleep(1 * time.Second)

		} else {
			commitChan <- msg
		}

	}
}
