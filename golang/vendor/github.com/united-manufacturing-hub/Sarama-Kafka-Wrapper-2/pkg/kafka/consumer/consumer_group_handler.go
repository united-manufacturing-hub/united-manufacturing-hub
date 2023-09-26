package consumer

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
	return nil
}

func (c *GroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	session.Commit()
	// Wait for one cycle to finish
	time.Sleep(shared.CycleTime)
	return nil
}

func (c *GroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// This must be smaller then Config.Consumer.Group.Rebalance.Timeout (default 60s)
	go consumer(&session, &claim, c.incomingMessages, c.running, c.consumedMessages)
	go marker(&session, c.messagesToMark, c.running, c.markedMessages)
	// Wait for c.running to be false
	var err error
	for c.running.Load() {
		time.Sleep(shared.CycleTime * 10)
	}
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
				(*session).Commit()
			}
		case <-time.After(shared.CycleTime):
			continue
		}
	}

	zap.S().Debugf("Committing messages")
	for k, v := range offsets {
		(*session).MarkOffset(k.Topic, k.Partition, v, "")
	}

	(*session).Commit()
}

func consumer(session *sarama.ConsumerGroupSession, claim *sarama.ConsumerGroupClaim, incomingMessages chan *shared.KafkaMessage, running *atomic.Bool, consumedMessages *atomic.Uint64) {
	timer := time.NewTimer(shared.CycleTime)
	messagesHandledCurrTenSeconds := 0.0
	for running.Load() {
		select {
		case message := <-(*claim).Messages():
			if session == nil {
				running.Store(false)
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
		}
	}
}
