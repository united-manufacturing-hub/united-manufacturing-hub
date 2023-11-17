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

func (c *GroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	zap.S().Debugf("Hello from setup")
	go marker(&session, c.messagesToMark, c.shutdownChannel, c.markedMessages)
	return nil
}

func (c *GroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	zap.S().Debugf("Begin cleanup")
	shutdown(c.shutdownChannel)
	// Wait for one cycle to finish
	zap.S().Debugf("Goodbye from cleanup")
	return nil
}

func (c *GroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	zap.S().Debugf("Begin ConsumeClaim")
	consumer(&session, &claim, c.incomingMessages, c.shutdownChannel, c.consumedMessages)
	zap.S().Debugf("Goodbye from consume claim (%d-%s)", session.GenerationID(), session.MemberID())
	return nil
}

type TopicPartition struct {
	Topic     string
	Partition int32
}

// marker marks messages coming in from messagesToMark to be ready for commit
func marker(session *sarama.ConsumerGroupSession, messagesToMark chan *shared.KafkaMessage, shutdownchan chan bool, markedMessages *atomic.Uint64) {
	zap.S().Debugf("begin marker")
	//lastMark := time.Now()
	//offsets := make(map[TopicPartition]int64)
	//lastLoopMarked := false
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
			(*session).MarkOffset(message.Topic, message.Partition, message.Offset+1, "")

			// Checks if there is already an offset for the given topic and partition
			// If the topic is new, then it can happen that there is no offset, but it should only happen once for each topic and partition and consumer group
			/*
				key := TopicPartition{
					Topic:     message.Topic,
					Partition: message.Partition,
				}
				if v, ok := offsets[key]; ok {
					if v < message.Offset {
						offsets[key] = message.Offset + 1
					}
				} else {
					offsets[key] = message.Offset + 1
				}
				markedMessages.Add(1)

				// If 10 seconds since the last comit happened or every 1000 messages, then
				if !lastLoopMarked && (markedMessages.Load()%1000 == 0 || time.Since(lastMark) > 10*time.Second) {
					zap.S().Debugf("Reached %d marked messages, committing", markedMessages.Load())
					lastMark = time.Now()
					for k, v := range offsets {
						(*session).MarkOffset(k.Topic, k.Partition, v, "")
					}
					clear(offsets)
					zap.S().Debugf("Marked offsets")
					lastLoopMarked = true
				}
			*/
		case <-(*session).Context().Done():
			zap.S().Debugf("Marker for session %s:%d is done", (*session).MemberID(), (*session).GenerationID())
			break outer
		}
	}

	zap.S().Debugf("Marker committing messages")
	/*
		for k, v := range offsets {
			(*session).MarkOffset(k.Topic, k.Partition, v, "")
		}*/
	zap.S().Debugf("Goodbye from marker (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}

func consumer(session *sarama.ConsumerGroupSession, claim *sarama.ConsumerGroupClaim, incomingMessages chan *shared.KafkaMessage, shutdownchan chan bool, consumedMessages *atomic.Uint64) {
	zap.S().Debugf("begin consumer %d:%s Messages: %d/%d", (*session).GenerationID(), (*session).MemberID(), len((*claim).Messages()), cap((*claim).Messages()))
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
				zap.S().Debugf("Message is nil for %s:%d", (*session).MemberID(), (*session).GenerationID())
				time.Sleep(shared.CycleTime)
				continue
			}
			// Add to incoming message channel, else block
			incomingMessages <- shared.FromConsumerMessage(message)
			consumedMessages.Add(1)
			messagesHandledCurrTenSeconds++
		case <-ticker10Seconds.C:
			msgPerSecond := messagesHandledCurrTenSeconds / 10
			zap.S().Debugf("Consumer for session %s:%d is active (%f msg/s) [Claims: %+v] [InitialOffset: %d], [HighWaterMarkOffset: %d]", (*session).MemberID(), (*session).GenerationID(), msgPerSecond, (*session).Claims(), (*claim).InitialOffset(), (*claim).HighWaterMarkOffset())
			if messagesHandledCurrTenSeconds == 0 {
				zap.S().Debugf("Handler got no messages, exiting")
				break outer
			}
			messagesHandledCurrTenSeconds = 0
			continue
		case <-(*session).Context().Done():
			msgPerSecond := messagesHandledCurrTenSeconds / 10
			zap.S().Debugf("Consumer for session %s:%d is done (%f msg/s) [Claims: %+v] [InitialOffset: %d], [HighWaterMarkOffset: %d]", (*session).MemberID(), (*session).GenerationID(), msgPerSecond, (*session).Claims(), (*claim).InitialOffset(), (*claim).HighWaterMarkOffset())
			break outer
		}
	}
	zap.S().Debugf("Goodbye from consumer (%d-%s)", (*session).GenerationID(), (*session).MemberID())
}
