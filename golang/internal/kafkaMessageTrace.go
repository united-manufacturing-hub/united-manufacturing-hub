// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"errors"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"time"
)

var SerialNumber, snErr = env.GetAsString("SERIAL_NUMBER", true, "")

var MicroserviceName, mnErr = env.GetAsString("MICROSERVICE_NAME", true, "")

type TraceValue struct {
	Traces map[int64]string `json:"trace"`
}

func Produce(producer *kafka.Producer, msg *kafka.Message, deliveryChan chan kafka.Event) error {
	if mnErr != nil {
		zap.S().Error(mnErr)
		return mnErr
	}
	if snErr != nil {
		zap.S().Error(snErr)
		return snErr
	}
	identifier := MicroserviceName + "-" + SerialNumber
	err := AddXTrace(msg, identifier)
	if err != nil {
		return err
	}
	return producer.Produce(msg, deliveryChan)
}

func addXOrigin(message *kafka.Message, origin string) error {
	return addHeaderTrace(message, "x-origin", origin)
}

func AddXOriginIfMissing(message *kafka.Message) error {
	trace := GetTrace(message, "x-origin")
	if trace == nil {
		err := addXOrigin(message, SerialNumber)
		return err
	}
	return nil
}

func AddXTrace(message *kafka.Message, value string) error {
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

func addHeaderTrace(message *kafka.Message, key, value string) error {
	if message.Headers == nil {
		message.Headers = make([]kafka.Header, 0)
	}

	for i := 0; i < len(message.Headers); i++ {
		header := message.Headers[i]
		if header.Key == key {
			// Json decode
			var traceValue TraceValue
			err := json.Unmarshal(header.Value, traceValue)
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
			var j []byte
			j, err = json.Marshal(traceValue)
			if err != nil {
				return err
			}
			// Update header
			header.Value = j
			message.Headers[i] = header
			return nil
		}
	}

	// Create new header
	var traceValue TraceValue
	traceValue.Traces = make(map[int64]string)
	traceValue.Traces[time.Now().UnixNano()] = value
	// Json encode
	j, err := json.Marshal(traceValue)
	if err != nil {
		return err
	}

	// Add new header
	message.Headers = append(message.Headers, kafka.Header{
		Key:   key,
		Value: j,
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
			err := json.Unmarshal(header.Value, &traceValue)
			if err != nil {
				zap.S().Errorf("Failed to unmarshal trace header: %s (%s)", err, key)
				return nil
			}
			return &traceValue
		}
	}
	return nil
}
