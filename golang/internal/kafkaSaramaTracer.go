package internal

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"time"
)

func AddSHeader(msg *kafka.Message, key string, value string) {
	if msg.Header == nil {
		msg.Header = map[string][]byte{}
	}
	msg.Header[key] = []byte(value)
}

func GetSHeader(msg *kafka.Message, key string) (bool, string) {
	if msg.Header == nil {
		return false, ""
	}
	if value, ok := msg.Header[key]; ok {
		return true, string(value)
	}
	return false, ""
}

func IsSameOrigin(msg *kafka.Message) bool {
	ok, origin := GetSXOrigin(msg)
	if !ok {
		return false
	}
	return origin == SerialNumber
}
func GetSXOrigin(msg *kafka.Message) (bool, string) {
	return GetSHeader(msg, "x-origin")
}

func AddSXOrigin(msg *kafka.Message) {
	AddSHeader(msg, "x-origin", SerialNumber)
}

func IsInTrace(msg *kafka.Message) bool {
	identifier := MicroserviceName + "-" + SerialNumber

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

func GetSXTrace(msg *kafka.Message) (bool, TraceValue) {
	ok, traceS := GetSHeader(msg, "x-trace")
	var traceValue TraceValue
	if !ok {
		return false, traceValue
	} else {
		err := jsoniter.Unmarshal([]byte(traceS), traceValue)
		if err != nil {
			return false, traceValue
		}
	}
	return true, traceValue
}

func AddSXTrace(msg *kafka.Message) error {
	identifier := MicroserviceName + "-" + SerialNumber
	ok, trace := GetSXTrace(msg)
	if !ok {
		trace = TraceValue{
			Traces: map[int64]string{},
		}
		trace.Traces = make(map[int64]string)
	}

	t := time.Now().UnixNano()
	trace.Traces[t] = identifier

	j, err := jsoniter.Marshal(trace)
	if err != nil {
		return err
	}
	AddSHeader(msg, "x-trace", string(j))

	return nil
}
