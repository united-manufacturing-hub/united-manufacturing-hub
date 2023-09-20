package kafka

import (
	"bytes"
	"errors"
	"time"

	"github.com/IBM/sarama"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

type TraceValue struct {
	Traces map[int64]string `json:"trace"`
}

func addTagsToHeaders(headers *[]sarama.RecordHeader) error {
	if !enableTagging {
		return nil
	}
	if headers == nil || len(*headers) == 0 {
		// If there are no headers, we need to create the slice
		*headers = make([]sarama.RecordHeader, 0, 1)
	}

	identifier := MicroserviceName + "-" + SerialNumber
	return AddXTrace(headers, identifier)
}

func addXOrigin(message *[]sarama.RecordHeader, origin string) error {
	return addHeaderTrace(message, "x-origin", origin)
}

func AddXOriginIfMissing(message *[]sarama.RecordHeader) error {
	trace := GetTrace(message, "x-origin")
	if trace == nil {
		err := addXOrigin(message, SerialNumber)
		return err
	}
	return nil
}

func AddXTrace(message *[]sarama.RecordHeader, value string) error {
	err := addHeaderTrace(message, "x-trace", value)
	if err != nil {
		return err
	}
	err = AddXOriginIfMissing(message)
	if err != nil {
		return err
	}
	return nil
}

func addHeaderTrace(message *[]sarama.RecordHeader, key, value string) error {
	if *message == nil {
		*message = make([]sarama.RecordHeader, 0)
	}

	for i := 0; i < len(*message); i++ {
		header := (*message)[i]
		if bytes.EqualFold(header.Key, []byte(key)) {
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
			jsonBytes, err := jsoniter.Marshal(traceValue)
			if err != nil {
				return err
			}
			// Update header
			header.Value = jsonBytes
			(*message)[i] = header
			return nil
		}
	}

	// Create new header
	var traceValue TraceValue
	traceValue.Traces = make(map[int64]string)
	traceValue.Traces[time.Now().UnixNano()] = value
	// Json encode
	jsonBytes, err := jsoniter.Marshal(traceValue)
	if err != nil {
		return err
	}

	// Add new header
	*message = append(*message, sarama.RecordHeader{
		Key:   []byte(key),
		Value: jsonBytes,
	})
	return nil
}

func GetTrace(message *[]sarama.RecordHeader, key string) *TraceValue {
	if message == nil {
		return nil
	}

	for i := 0; i < len(*message); i++ {
		header := (*message)[i]
		if bytes.EqualFold(header.Key, []byte(key)) {
			// Json decode
			var traceValue TraceValue
			err := jsoniter.Unmarshal(header.Value, &traceValue)
			if err != nil {
				zap.S().Errorf("Failed to unmarshal trace header: %s (%s)", err, key)
				return nil
			}
			return &traceValue
		}
	}
	return nil
}
