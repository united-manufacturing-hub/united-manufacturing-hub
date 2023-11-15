package worker

import (
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
	"strconv"
	"sync"
)

type Worker struct {
	kafka    *kafka.Connection
	postgres *postgresql.Connection
}

var worker *Worker
var once sync.Once

func Init() *Worker {
	once.Do(func() {
		worker = &Worker{
			kafka:    kafka.Init(),
			postgres: postgresql.Init(),
		}
		go worker.startWorkLoop()
	})
	return worker
}

func (w *Worker) startWorkLoop() {
	zap.S().Debugf("Started work loop")
	messageChannel := w.kafka.GetMessages()
	for {
		msg := <-messageChannel
		zap.S().Debugf("Got message: %+v", msg)
		topic, err := recreateTopic(msg)
		if err != nil {
			zap.S().Warnf("Failed to parse message %+v into topic: %s", msg, err)
			w.kafka.MarkMessage(msg)
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
			payload, timestampMs, err := parseHistorianPayload(msg.Value)
			if err != nil {
				zap.S().Warnf("Failed to parse payload %+v for message: %s ", msg, err)
				continue
			}
			zap.S().Debug("payload:%s", payload)
			err = w.postgres.InsertHistorianValue(payload, timestampMs, origin, topic, payload.Name)
			if err != nil {
				zap.S().Warnf("Failed to insert historian numerical value %+v: %s", msg, err)
				continue
			}

		case "analytics":
			zap.S().Warnf("Analytics not yet supported, ignoring")
		}
		w.kafka.MarkMessage(msg)

	}
}

func parseHistorianPayload(value []byte) (*sharedStructs.Value, int64, error) {
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
	var v *sharedStructs.Value
	var vFound bool

	for key, value := range message {
		var parsed *sharedStructs.Value
		parsed, err = parseValue(value)
		if err != nil {
			return nil, 0, err
		}
		if key == "timestamp_ms" {
			if !parsed.IsNumeric {
				return nil, 0, fmt.Errorf("expected timestamp_ms to be numeric, got: %+v", parsed)
			}
			timestampMs = int64(*parsed.NumericValue)
			timestampFound = true
		} else {
			v = parsed
			vFound = true
		}
	}

	if !timestampFound {
		return nil, 0, fmt.Errorf("message value does not contain timestamp_ms: %+v", message)
	}
	if !vFound {
		return nil, 0, fmt.Errorf("message does not contain any value: %+v", message)
	}

	return v, timestampMs, nil
}

func parseValue(v interface{}) (*sharedStructs.Value, error) {
	var val sharedStructs.Value

	switch t := v.(type) {
	case float64:
		val.NumericValue = &t
		val.IsNumeric = true
	case int:
		f := float64(t)
		val.NumericValue = &f
		val.IsNumeric = true
	case string:
		if num, err := strconv.ParseFloat(t, 64); err == nil {
			n := num
			val.NumericValue = &n
			val.IsNumeric = true
		} else {
			val.StringValue = &t
		}
	default:
		return nil, fmt.Errorf("Unsupported type: %T (%v)", t, v)
	}

	return &val, nil
}
