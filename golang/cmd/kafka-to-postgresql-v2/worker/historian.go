package worker

import (
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"strings"
)

func parseHistorianPayload(value []byte, tag string) ([]sharedStructs.HistorianValue, int64, error) {
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
	var values = make([]sharedStructs.HistorianValue, 0)

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

	// Replace separators in the tag with $
	tag = strings.ReplaceAll(tag, ".", sharedStructs.DbTagSeparator)

	// Recursively parse the remaining fields
	err = parseValue(tag, message, &values)

	return values, timestampMs, err
}

func parseInt(v interface{}) (int64, error) {
	timestamp, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("timestamp_ms is not an float64: %T (%v)", v, v)
	}
	return int64(timestamp), nil
}

func parseValue(prefix string, v interface{}, values *[]sharedStructs.HistorianValue) (err error) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, v := range val {
			fullKey := k
			if prefix != "" {
				// Handle duplicate tag groups
				if strings.HasSuffix(prefix, k) {
					fullKey = prefix
				} else {
					fullKey = prefix + sharedStructs.DbTagSeparator + k
				}
			}
			err = parseValue(fullKey, v, values)
		}
	case float64:
		f := float32(val)
		*values = append(*values, sharedStructs.HistorianValue{
			Name:         prefix,
			NumericValue: &f,
			IsNumeric:    true,
		})
	case float32:
		*values = append(*values, sharedStructs.HistorianValue{
			Name:         prefix,
			NumericValue: &val,
			IsNumeric:    true,
		})
	case int:
		f := float32(val)
		*values = append(*values, sharedStructs.HistorianValue{
			Name:         prefix,
			NumericValue: &f,
			IsNumeric:    true,
		})
	case string:
		*values = append(*values, sharedStructs.HistorianValue{
			Name:        prefix,
			StringValue: &val,
		})
	case bool:
		f := float32(0.0)
		if val {
			f = 1.0
		}
		*values = append(*values, sharedStructs.HistorianValue{
			Name:         prefix,
			NumericValue: &f,
			IsNumeric:    true,
		})
	default:
		return fmt.Errorf("unsupported type %T (%v) for tag %s", val, val, prefix)
	}
	return err
}
