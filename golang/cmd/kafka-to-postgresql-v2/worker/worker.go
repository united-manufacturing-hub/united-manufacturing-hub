package worker

import (
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
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
	messageChannel := w.kafka.GetMessages()
	for {
		select {
		case msg := <-messageChannel:
			topic, err := recreateTopic(msg)
			if err != nil {
				zap.S().Warnf("Failed to parse message %+v into topic: %s", msg, err)
				w.kafka.MarkMessage(msg)
				continue
			}
			if topic == nil {
				zap.S().Fatalf("topic is null, after successfull parsing, this should never happen !: %+v", msg)
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
				if payload.IsNumeric {
					err := w.postgres.InsertHistorianNumericValue(payload.NumericValue, timestampMs, origin, topic, payload.Name)
					if err != nil {
						zap.S().Warnf("Failed to insert historian numerical value %+v: %s", msg, err)
						continue
					}
				}
			case "analytics":
				zap.S().Warnf("Analytics not yet supported, ignoring")
			}
			w.kafka.MarkMessage(msg)
		}
	}
}

type Value struct {
	NumericValue int64
	StringValue  string
	IsNumeric    bool
	Name         string
}

func parseHistorianPayload(value []byte) (*Value, int64, error) {
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
	var v Value
	var vFound bool

	for key, value := range message {
		if key == "timestamp_ms" {
			timestampMs, err = interfaceToInt(value)
			if err != nil {
				return nil, 0, err
			}
			timestampFound = true
			if timestampMs < 0 {
				zap.S().Warnf("message contains negative timestamp, this can be dangerous: %+v", message)
			}
		} else {
			v.Name = key
			var vX int64
			vX, err = interfaceToInt(value)
			if err != nil {
				var ok bool
				v.StringValue, ok = value.(string)
				if !ok {
					return nil, 0, fmt.Errorf("message value is not parseable as either int or string: %+v", message)
				}
				vFound = true
			} else {
				v.NumericValue = vX
				v.IsNumeric = true
				vFound = true
			}
		}
	}

	if !timestampFound {
		return nil, 0, fmt.Errorf("message value does not contain timestamp_ms: %+v", message)
	}
	if !vFound {
		return nil, 0, fmt.Errorf("message does not contain any value: %+v", message)
	}

	return &v, timestampMs, nil
}

func interfaceToInt(value interface{}) (int64, error) {
	// Attempt to directly cast to int
	var err error
	numericValue, ok := value.(int64)
	if !ok {
		timestampMsString, ok := value.(string)
		if !ok {
			return 0, fmt.Errorf("message value for timestamp_ms is neither string nor integer, but: %+v", value)
		}
		numericValue, err = strconv.ParseInt(timestampMsString, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return numericValue, err
}
