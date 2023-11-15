package redpanda

import (
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"sync/atomic"
	"time"
)

type GroupHandler struct {
	running          *atomic.Bool
	markedMessages   *atomic.Uint64
	consumedMessages *atomic.Uint64
	incomingMessages chan *shared.KafkaMessage
	messagesToMark   chan *shared.KafkaMessage
}

func (c *GroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	zap.S().Debugf("Hello from setup")
	c.running.Store(true)
	return nil
}

func commit(session sarama.ConsumerGroupSession) chan bool {
	now := time.Now()
	zap.S().Debugf("Committing messages")
	session.Commit()
	zap.S().Debugf("Commit took %s", time.Since(now))
	return nil
}

func (c *GroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	timeout := time.NewTimer(30 * time.Second)

	select {
	case <-timeout.C:
		zap.S().Debugf("Timeout reached, closing consumer")
		c.running.Store(false)
		return nil
	case <-commit(session):
		zap.S().Debugf("Cleanup commit finished")
	}

	c.running.Store(false)
	// Wait for one cycle to finish
	time.Sleep(100 * time.Millisecond)
	zap.S().Debugf("Goodbye from cleanup")
	return nil
}

func (c *GroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// This must be smaller then Config.Consumer.Group.Rebalance.Timeout (default 60s)
	go consumer(&session, &claim, c.incomingMessages, c.running, c.consumedMessages)
	go marker(&session, c.messagesToMark, c.running, c.markedMessages)
	// Wait for c.running to be false
	var err error
	for c.running.Load() {
		time.Sleep(100 * time.Millisecond * 10)
	}
	zap.S().Debugf("Goodbye from consume claim (%d-%s)", session.GenerationID(), session.MemberID())
	return err
}

type TopicPartition struct {
	Topic     string
	Partition int32
}

func marker(session *sarama.ConsumerGroupSession, messagesToMark chan *shared.KafkaMessage, running *atomic.Bool, markedMessages *atomic.Uint64) {
	lastCommit := time.Now()
	offsets := make(map[TopicPartition]int64)
	for running.Load() {
		select {
		case message := <-messagesToMark:
			if session == nil || (*session) == nil {
				running.Store(false)
				continue
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

			if markedMessages.Load()%10000 == 0 || time.Since(lastCommit) > 10*time.Second {
				lastCommit = time.Now()
				for k, v := range offsets {
					(*session).MarkOffset(k.Topic, k.Partition, v, "")
				}
				commit(*session)
			}
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}

	zap.S().Debugf("Marker committing messages")
	for k, v := range offsets {
		(*session).MarkOffset(k.Topic, k.Partition, v, "")
	}
	commit(*session)
	zap.S().Debugf("Goodbye from marker (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}

func consumer(session *sarama.ConsumerGroupSession, claim *sarama.ConsumerGroupClaim, incomingMessages chan *shared.KafkaMessage, running *atomic.Bool, consumedMessages *atomic.Uint64) {
	timer := time.NewTimer(100 * time.Millisecond)
	timerTenSeconds := time.NewTimer(10 * time.Second)
	messagesHandledCurrTenSeconds := 0.0
	for running.Load() {
		select {
		case message := <-(*claim).Messages():
			if session == nil {
				running.Store(false)
			}
			if message == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			// Add to incoming message channel, else block
			incomingMessages <- shared.FromConsumerMessage(message)
			consumedMessages.Add(1)
			messagesHandledCurrTenSeconds++
		case <-timer.C:
			timer.Reset(100 * time.Millisecond)
			continue
		case <-timerTenSeconds.C:
			zap.S().Debugf("Consumer for session %s:%d is running", (*session).MemberID(), (*session).GenerationID())
			continue
		}
	}
	zap.S().Debugf("Goodbye from consumer (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}
