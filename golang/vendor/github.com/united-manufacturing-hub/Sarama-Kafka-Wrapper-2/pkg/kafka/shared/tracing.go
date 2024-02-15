package shared

import (
	"encoding/json"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"time"
)

// serialNumber's error is ignored because it will fall back to the default value
// microserviceName's error is ignored because it will fall back to the default value
var (
	serialNumber, _     = env.GetAsString("SERIAL_NUMBER", false, "")     //nolint:errcheck
	microserviceName, _ = env.GetAsString("MICROSERVICE_NAME", false, "") //nolint:errcheck
)

type TraceValue struct {
	Traces map[int64]string `json:"trace"`
}

func AddSHeader(msg *KafkaMessage, key string, value string) {
	if msg.Headers == nil {
		msg.Headers = make(map[string]string)
	}
	msg.Headers[key] = value
}

func GetSHeader(msg *KafkaMessage, key string) (bool, string) {
	if msg.Headers == nil {
		return false, ""
	}
	if value, ok := msg.Headers[key]; ok {
		return true, value
	}
	return false, ""
}

func IsSameOrigin(msg *KafkaMessage) bool {
	ok, origin := GetSXOrigin(msg)
	if !ok {
		return false
	}
	return origin == serialNumber
}
func GetSXOrigin(msg *KafkaMessage) (bool, string) {
	return GetSHeader(msg, "x-origin")
}

func AddSXOrigin(msg *KafkaMessage) {
	AddSHeader(msg, "x-origin", serialNumber)
}

func IsInTrace(msg *KafkaMessage) bool {
	identifier := microserviceName + "-" + serialNumber

	ok, trace := GetSXTrace(msg)
	if !ok {
		return false
	}
	for _, s := range trace.Traces {
		if s == identifier {
			return true
		}
	}
	return false
}

func GetSXTrace(msg *KafkaMessage) (bool, TraceValue) {
	ok, traceS := GetSHeader(msg, "x-trace")
	var traceValue TraceValue
	if !ok {
		return false, traceValue
	} else {
		err := json.Unmarshal([]byte(traceS), &traceValue)
		if err != nil {
			return false, traceValue
		}
	}
	return true, traceValue
}

func AddSXTrace(msg *KafkaMessage) error {
	identifier := microserviceName + "-" + serialNumber
	ok, trace := GetSXTrace(msg)
	if !ok {
		trace = TraceValue{
			Traces: map[int64]string{},
		}
		trace.Traces = make(map[int64]string)
	}

	t := time.Now().UnixNano()
	trace.Traces[t] = identifier

	j, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	AddSHeader(msg, "x-trace", string(j))

	return nil
}
