package internal

import (
	"errors"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"time"
)

type TraceValue struct {
	Traces map[int64]string `json:"trace"`
}

func Produce(producer *kafka.Producer, msg *kafka.Message, deliveryChan chan kafka.Event, identifier string) error {
	err := AddTrace(msg, "x-trace", identifier)
	if err != nil {
		return err
	}
	return producer.Produce(msg, deliveryChan)
}

func AddTrace(message *kafka.Message, key, value string) error {
	if message.Headers == nil {
		message.Headers = make([]kafka.Header, 0)
	}

	for i := 0; i < len(message.Headers); i++ {
		header := message.Headers[i]
		if header.Key == key {
			// Json decode
			var traceValue TraceValue
			err := jsoniter.Unmarshal(header.Value, traceValue)
			if err != nil {
				return err
			}
			// Current time
			t := time.Now().UnixNano()
			// Check if trace already exists
			if _, ok := traceValue.Traces[t]; ok {
				return errors.New("trace already exists")
			}
			// Add new trace
			traceValue.Traces[t] = value
			// Json encode
			var json []byte
			json, err = jsoniter.Marshal(traceValue)
			if err != nil {
				return err
			}
			// Update header
			header.Value = json
			message.Headers[i] = header
			return nil
		}
	}

	// Create new header
	var traceValue TraceValue
	traceValue.Traces = make(map[int64]string)
	traceValue.Traces[time.Now().UnixNano()] = value
	// Json encode
	var json []byte
	json, err := jsoniter.Marshal(traceValue)
	if err != nil {
		return err
	}

	// Add new header
	message.Headers = append(message.Headers, kafka.Header{
		Key:   key,
		Value: json,
	})
	return nil
}

func GetTrace(message *kafka.Message, key string) *TraceValue {
	if message.Headers == nil {
		return nil
	}

	for i := 0; i < len(message.Headers); i++ {
		header := message.Headers[i]
		if header.Key == key {
			// Json decode
			var traceValue TraceValue
			err := jsoniter.Unmarshal(header.Value, traceValue)
			if err != nil {
				return nil
			}
			return &traceValue
		}
	}
	return nil
}
