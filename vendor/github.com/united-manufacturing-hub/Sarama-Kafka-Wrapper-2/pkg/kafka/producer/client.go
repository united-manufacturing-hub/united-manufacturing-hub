package producer

import (
	"github.com/IBM/sarama"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"go.uber.org/zap"
	"sync/atomic"
	"time"
)

// Producer struct wraps a sarama.AsyncProducer and handles Kafka message production.
type Producer struct {
	producer         *sarama.AsyncProducer
	brokers          []string
	producedMessages atomic.Uint64
	erroredMessages  atomic.Uint64
	running          atomic.Bool
}

// NewProducer creates a new Producer with the given Kafka brokers.
func NewProducer(brokers []string) (*Producer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = false
	config.Producer.Return.Errors = true

	producer, err := sarama.NewAsyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	p := &Producer{
		brokers:  brokers,
		producer: &producer,
	}
	p.running.Store(true)
	go p.handleErrors()

	return p, nil
}

// handleErrors handles errors from the producer in a goroutine.
func (p *Producer) handleErrors() {
	timeout := time.NewTimer(shared.CycleTime)
	defer timeout.Stop()

	for p.running.Load() {
		select {
		case <-timeout.C:
			timeout.Reset(shared.CycleTime)
		case err := <-(*p.producer).Errors():
			if err != nil {
				p.erroredMessages.Add(1)
				zap.S().Debugf("Error while producing message: %s", err.Error())
			}
		}
	}
}

// SendMessage sends a KafkaMessage to the producer.
func (p *Producer) SendMessage(message *shared.KafkaMessage) {
	if message == nil {
		return
	}
	(*p.producer).Input() <- shared.ToProducerMessage(message)
	p.producedMessages.Add(1)
}

// Close stops the producer and returns any errors during closure.
func (p *Producer) Close() error {
	p.running.Store(false)
	return (*p.producer).Close()
}

// GetProducedMessages returns the count of produced and errored messages.
func (p *Producer) GetProducedMessages() (uint64, uint64) {
	return p.producedMessages.Load(), p.erroredMessages.Load()
}
