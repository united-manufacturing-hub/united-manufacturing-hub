package redpanda

import (
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"sync/atomic"
	"time"
)

type GroupHandler struct {
	shutdownChannel  chan bool
	markedMessages   *atomic.Uint64
	consumedMessages *atomic.Uint64
	incomingMessages chan *shared.KafkaMessage
	messagesToMark   chan *shared.KafkaMessage
}

func (c *GroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	zap.S().Debugf("Hello from setup")
	return nil
}

func (c *GroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	zap.S().Debugf("Begin cleanup")
	shutdown(c.shutdownChannel)
	// Wait for one cycle to finish
	time.Sleep(shared.CycleTime)
	zap.S().Debugf("Goodbye from cleanup")
	return nil
}

func (c *GroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	zap.S().Debugf("Begin ConsumeClaim")
	// This must be smaller then Config.Consumer.Group.Rebalance.Timeout (default 60s)
	go consumer(&session, &claim, c.incomingMessages, c.shutdownChannel, c.consumedMessages)
	go marker(&session, c.messagesToMark, c.shutdownChannel, c.markedMessages)
	// Wait for c.shutdownChannel to have a value
	<-c.shutdownChannel
	zap.S().Debugf("Goodbye from consume claim (%d-%s)", session.GenerationID(), session.MemberID())
	return nil
}

type TopicPartition struct {
	Topic     string
	Partition int32
}

func marker(session *sarama.ConsumerGroupSession, messagesToMark chan *shared.KafkaMessage, shutdownchan chan bool, markedMessages *atomic.Uint64) {
	zap.S().Debugf("begin marker")
	lastCommit := time.Now()
	offsets := make(map[TopicPartition]int64)
	lastLoopCommited := false
outer:
	for {
		select {
		case <-shutdownchan:
			break outer
		case message := <-messagesToMark:
			if session == nil || (*session) == nil {
				shutdown(shutdownchan)
				break
			}
			if message == nil {
				continue
			}

			key := TopicPartition{
				Topic:     message.Topic,
				Partition: message.Partition,
			}
			if v, ok := offsets[key]; ok {
				if v < message.Offset {
					offsets[key] = message.Offset
				}
			} else {
				offsets[key] = message.Offset
			}
			markedMessages.Add(1)

			if !lastLoopCommited && (markedMessages.Load()%1000 == 0 || time.Since(lastCommit) > 10*time.Second) {
				zap.S().Debugf("Reached %d marked messages, committing", markedMessages.Load())
				lastCommit = time.Now()
				for k, v := range offsets {
					(*session).MarkOffset(k.Topic, k.Partition, v, "")
				}
				clear(offsets)
				zap.S().Debugf("Marked offsets")
				lastLoopCommited = true
			}
		case <-time.After(shared.CycleTime):
			continue
		case <-(*session).Context().Done():
			zap.S().Debugf("Marker for session %s:%d is done", (*session).MemberID(), (*session).GenerationID())
			break outer
		}
	}

	zap.S().Debugf("Marker committing messages")
	for k, v := range offsets {
		(*session).MarkOffset(k.Topic, k.Partition, v, "")
	}
	zap.S().Debugf("Goodbye from marker (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}

func consumer(session *sarama.ConsumerGroupSession, claim *sarama.ConsumerGroupClaim, incomingMessages chan *shared.KafkaMessage, shutdownchan chan bool, consumedMessages *atomic.Uint64) {
	zap.S().Debugf("begin consumer")
	ticker := time.NewTicker(shared.CycleTime)
	ticker10Seconds := time.NewTicker(10 * time.Second)
	messagesHandledCurrTenSeconds := 0.0
outer:
	for {
		select {
		case <-shutdownchan:
			break outer
		case message := <-(*claim).Messages():
			if session == nil {
				shutdown(shutdownchan)
				continue
			}
			if message == nil {
				time.Sleep(shared.CycleTime)
				continue
			}
			// Add to incoming message channel, else block
			incomingMessages <- shared.FromConsumerMessage(message)
			consumedMessages.Add(1)
			messagesHandledCurrTenSeconds++
		case <-ticker.C:
			continue
		case <-ticker10Seconds.C:
			msgPerSecond := messagesHandledCurrTenSeconds / 10
			zap.S().Debugf("Consumer for session %s:%d is active (%f msg/s) [Claims: %+v] [InitialOffset: %d], [HighWaterMarkOffset: %d]", (*session).MemberID(), (*session).GenerationID(), msgPerSecond, (*session).Claims(), (*claim).InitialOffset(), (*claim).HighWaterMarkOffset())
			messagesHandledCurrTenSeconds = 0
			continue
		case <-(*session).Context().Done():
			zap.S().Debugf("Consumer for session %s:%d is done", (*session).MemberID(), (*session).GenerationID())
			break outer
		}
	}
	zap.S().Debugf("Goodbye from consumer (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}
