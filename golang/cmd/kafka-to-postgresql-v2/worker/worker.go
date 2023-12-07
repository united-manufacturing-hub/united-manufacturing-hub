package worker

import (
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
	"sync"
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

		switch topic.Usecase {
		case "historian":
			payloads, timestampMs, err := parseHistorianPayload(msg.Value)
			if err != nil {
				zap.S().Warnf("Failed to parse payload %+v for message: %s ", msg, err)
				k.MarkMessage(msg)
				continue
			}
			err = p.InsertHistorianValue(payloads, timestampMs, origin, topic)
			if err != nil {
				zap.S().Warnf("Failed to insert historian numerical value %+v: %s [%+v]", msg, err, payloads)
				k.MarkMessage(msg)
				continue
			}
		case "analytics":
			zap.S().Warnf("Analytics not yet supported, ignoring")
		default:
			zap.S().Errorf("Unknown usecase %s", topic.Usecase)
		}
		k.MarkMessage(msg)
	}
}

func parseHistorianPayload(value []byte) ([]sharedStructs.Value, int64, error) {
	// Attempt to JSON decode the message
	var message map[string]interface{}
	err := json.Unmarshal(value, &message)
	if err != nil {
		return nil, 0, err
	}
	// There should only be two fields and one of them is "timestamp_ms"
	if len(message) != 2 {
		return nil, 0, errors.New("message contains does not have exactly 2 fields")
	}
	var timestampMs int64
	var timestampFound bool
	var values []sharedStructs.Value
	var vFound bool

	for key, value := range message {
		if key == "timestamp_ms" {
			timestampMs, err = parseInt(value)
			if err != nil {
				return nil, 0, err
			}
			timestampFound = true
		} else {
			var v sharedStructs.Value
			v, err = parseValue(value)
			if err != nil {
				return nil, 0, err
			}
			vFound = true
			v.Name = key
			values = append(values, v)
		}
	}

	if !timestampFound {
		return nil, 0, fmt.Errorf("message value does not contain timestamp_ms: %+v", message)
	}
	if !vFound {
		return nil, 0, fmt.Errorf("message does not contain any value: %+v", message)
	}

	return values, timestampMs, nil
}

func parseInt(v interface{}) (int64, error) {
	timestamp, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("timestamp_ms is not an float64: %T (%v)", v, v)
	}
	return int64(timestamp), nil
}

func parseValue(v interface{}) (sharedStructs.Value, error) {
	var val sharedStructs.Value
	var numericVal float32

	switch t := v.(type) {
	case float64:
		numericVal = float32(t)
		val.NumericValue = &numericVal
		val.IsNumeric = true
	case string:
		val.StringValue = &t
	case float32:
		numericVal = t
		val.NumericValue = &numericVal
		val.IsNumeric = true
	case int:
		numericVal = float32(t)
		val.NumericValue = &numericVal
		val.IsNumeric = true
	case bool:
		numericVal = 0.0
		if t {
			numericVal = 1.0
		}
		val.NumericValue = &numericVal
	default:
		return sharedStructs.Value{}, fmt.Errorf("unsupported type: %T (%v)", t, v)
	}

	return val, nil
}
