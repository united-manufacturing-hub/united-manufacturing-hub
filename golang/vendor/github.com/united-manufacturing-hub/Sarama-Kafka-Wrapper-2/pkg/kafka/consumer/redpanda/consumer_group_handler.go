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

func commit(session sarama.ConsumerGroupSession) chan bool {
	now := time.Now()
	zap.S().Debugf("Committing messages")
	session.Commit()
	zap.S().Debugf("Commit took %s", time.Since(now))
	return nil
}

func commitWithTimeout(session sarama.ConsumerGroupSession, shutdownchannel chan bool) {
	timeout := time.NewTimer(30 * time.Second)
	select {
	case <-timeout.C:
		zap.S().Debugf("Timeout reached, closing consumer")
		shutdown(shutdownchannel)
	case <-commit(session):
		zap.S().Debugf("Cleanup commit finished")
	}
}

func (c *GroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	commitWithTimeout(session, c.shutdownChannel)
	shutdown(c.shutdownChannel)
	// Wait for one cycle to finish
	time.Sleep(shared.CycleTime)
	zap.S().Debugf("Goodbye from cleanup")
	return nil
}

func (c *GroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
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
	lastCommit := time.Now()
	offsets := make(map[TopicPartition]int64)
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

			if markedMessages.Load()%1000 == 0 || time.Since(lastCommit) > 10*time.Second {
				lastCommit = time.Now()
				for k, v := range offsets {
					(*session).MarkOffset(k.Topic, k.Partition, v, "")
				}
				zap.S().Debugf("Reached %d marked messages, committing", markedMessages.Load())
				commitWithTimeout(*session, shutdownchan)
			}
		case <-time.After(shared.CycleTime):
			continue
		}
	}

	zap.S().Debugf("Marker committing messages")
	for k, v := range offsets {
		(*session).MarkOffset(k.Topic, k.Partition, v, "")
	}
	commitWithTimeout(*session, shutdownchan)
	zap.S().Debugf("Goodbye from marker (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}

func consumer(session *sarama.ConsumerGroupSession, claim *sarama.ConsumerGroupClaim, incomingMessages chan *shared.KafkaMessage, shutdownchan chan bool, consumedMessages *atomic.Uint64) {
	timer := time.NewTimer(shared.CycleTime)
	timerTenSeconds := time.NewTimer(10 * time.Second)
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
		case <-timer.C:
			timer.Reset(shared.CycleTime)
			continue
		case <-timerTenSeconds.C:
			zap.S().Debugf("Consumer for session %s:%d is shutdownChannel", (*session).MemberID(), (*session).GenerationID())
			continue
		}
	}
	zap.S().Debugf("Goodbye from consumer (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}
