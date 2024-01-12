package worker

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
)

type Worker struct {
	kafka    *kafka.Connection
	postgres *postgresql.Connection
}

var worker *Worker
var once sync.Once

func GetOrInit() *Worker {
	once.Do(func() {
		worker = &Worker{
			kafka:    kafka.GetOrInit(),
			postgres: postgresql.GetOrInit(),
		}
		worker.startWorkLoop()
	})
	return worker
}

func (w *Worker) startWorkLoop() {
	zap.S().Debugf("Started work loop")
	workerMultiplier, err := env.GetAsInt("WORKER_MULTIPLIER", false, 16)
	if err != nil {
		zap.S().Fatalf("Failed to get WORKER_MULTIPLIER from env: %s", err)
	}
	messageChannel := w.kafka.GetMessages()
	zap.S().Debugf("Started using %d workers (logical cores * WORKER_MULTIPLIER)", workerMultiplier)
	for i := 0; i < /*runtime.NumCPU()*workerMultiplier*/ 1; i++ {
		go handleParsing(messageChannel, i)
	}
	zap.S().Debugf("Started all workers")
}

func handleParsing(msgChan <-chan *shared.KafkaMessage, i int) {
	k := kafka.GetOrInit()
	p := postgresql.GetOrInit()
	messagesHandled := 0
	now := time.Now()
	for {
		msg := <-msgChan
		topic, err := recreateTopic(msg)
		if err != nil {
			zap.S().Warnf("Failed to parse message %+v into topic: %s", msg, err)
			k.MarkMessage(msg)
			continue
		}
		if topic == nil {
			zap.S().Fatalf("topic is null, after successful parsing, this should never happen !: %+v", msg)
		}

		origin, hasOrigin := msg.Headers["x-origin"]
		if !hasOrigin {
			origin = "unknown"
		}

		switch topic.Schema {
		case "historian":
			payload, timestampMs, err := parseHistorianPayload(msg.Value, topic.Tag)
			if err != nil {
				zap.S().Warnf("Failed to parse payload %+v for message: %s ", msg, err)
				k.MarkMessage(msg)
				continue
			}
			err = p.InsertHistorianValue(payload, timestampMs, origin, topic)
			if err != nil {
				zap.S().Warnf("Failed to insert historian numerical value %+v: %s [%+v]", msg, err, payload)
				k.MarkMessage(msg)
				continue
			}
		case "analytics":
			zap.S().Warnf("Analytics not yet supported, ignoring")
		default:
			zap.S().Errorf("Unknown usecase %s", topic.Schema)
		}
		k.MarkMessage(msg)
		messagesHandled++
		elapsed := time.Since(now)
		if int(elapsed.Seconds())%10 == 0 {
			zap.S().Debugf("handleParsing [%d] handled %d messages in %s (%f msg/s) [%d/%d]", i, messagesHandled, elapsed, float64(messagesHandled)/elapsed.Seconds(), len(msgChan), cap(msgChan))
		}
	}
}

func parseHistorianPayload(value []byte, tag string) ([]sharedStructs.Value, int64, error) {
	// Attempt to JSON decode the message
	var message map[string]interface{}
	err := json.Unmarshal(value, &message)
	if err != nil {
		return nil, 0, err
	}
	// The payload must contain at least 2 fields: timestamp_ms and a value
	if len(message) < 2 {
		return nil, 0, errors.New("message payload does not contain enough fields")
	}
	var timestampMs int64
	var values = make([]sharedStructs.Value, 0)

	// Extract and remove the timestamp_ms field
	if ts, ok := message["timestamp_ms"]; !ok {
		return nil, 0, errors.New("message value does not contain timestamp_ms")
	} else {
		timestampMs, err = parseInt(ts)
		if err != nil {
			return nil, 0, err
		}
		delete(message, "timestamp_ms")
	}

	// Recursively parse the remaining fields
	parseValue(tag, message, &values)

	return values, timestampMs, nil
}

func parseInt(v interface{}) (int64, error) {
	timestamp, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("timestamp_ms is not an float64: %T (%v)", v, v)
	}
	return int64(timestamp), nil
}

func parseValue(prefix string, v interface{}, values *[]sharedStructs.Value) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, v := range val {
			fullKey := k
			if prefix != "" {
				if strings.HasSuffix(prefix, k) {
					fullKey = prefix
				} else {
					fullKey = prefix + "." + k
				}
			}
			parseValue(fullKey, v, values)
		}
	case float64:
		f := float32(val)
		*values = append(*values, sharedStructs.Value{
			Name:         prefix,
			NumericValue: &f,
			IsNumeric:    true,
		})
	case float32:
		*values = append(*values, sharedStructs.Value{
			Name:         prefix,
			NumericValue: &val,
			IsNumeric:    true,
		})
	case int:
		f := float32(val)
		*values = append(*values, sharedStructs.Value{
			Name:         prefix,
			NumericValue: &f,
			IsNumeric:    true,
		})
	case string:
		*values = append(*values, sharedStructs.Value{
			Name:        prefix,
			StringValue: &val,
		})
	case bool:
		f := float32(0.0)
		if val {
			f = 1.0
		}
		*values = append(*values, sharedStructs.Value{
			Name:         prefix,
			NumericValue: &f,
			IsNumeric:    true,
		})
	default:
		zap.S().Warnf("Unsupported type %T (%v) for tag %s", val, val, prefix)
	}
}
