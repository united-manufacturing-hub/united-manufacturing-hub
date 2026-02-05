// Copyright 2025 UMH Systems GmbH
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

package deps

import "time"

// FSMLogger provides structured logging with compile-time enforcement of Sentry fields.
//
// Debug and Info are general-purpose. All warn/error level logs require a Feature
// parameter for Sentry routing — there is no bare Warn() method. This ensures every
// warning and error reaches Sentry with a proper feature tag.
type FSMLogger interface {
	// Debug logs at DEBUG level with structured fields.
	Debug(msg string, fields ...Field)

	// Info logs at INFO level with structured fields.
	Info(msg string, fields ...Field)

	// SentryWarn logs at WARN level with required Feature for Sentry routing.
	// The feature parameter identifies which subsystem owns this warning.
	SentryWarn(feature Feature, msg string, fields ...Field)

	// SentryError logs at ERROR level with required Feature and error for Sentry.
	// The feature parameter identifies which subsystem owns this error.
	// The err parameter is captured as a Sentry exception with stack trace.
	SentryError(feature Feature, err error, msg string, fields ...Field)

	// With returns a new FSMLogger with additional context fields.
	// All logs from the returned logger include these fields.
	With(fields ...Field) FSMLogger
}

// Field represents a structured log field as a key-value pair.
type Field struct {
	Value any
	Key   string
}

// String creates a Field with a string value.
func String(key, val string) Field {
	return Field{Key: key, Value: val}
}

// Int creates a Field with an int value.
func Int(key string, val int) Field {
	return Field{Key: key, Value: val}
}

// Int64 creates a Field with an int64 value.
func Int64(key string, val int64) Field {
	return Field{Key: key, Value: val}
}

// Float64 creates a Field with a float64 value.
func Float64(key string, val float64) Field {
	return Field{Key: key, Value: val}
}

// Bool creates a Field with a bool value.
func Bool(key string, val bool) Field {
	return Field{Key: key, Value: val}
}

// Duration creates a Field with a time.Duration value.
func Duration(key string, val time.Duration) Field {
	return Field{Key: key, Value: val}
}

// Time creates a Field with a time.Time value.
func Time(key string, val time.Time) Field {
	return Field{Key: key, Value: val}
}

// Any creates a Field with any value. Use typed constructors when possible.
func Any(key string, val any) Field {
	return Field{Key: key, Value: val}
}

// Err creates a Field with an error value using the standard "error" key.
// Use this for additional errors beyond the primary error in SentryError.
func Err(err error) Field {
	return Field{Key: "error", Value: err}
}

// HierarchyPath creates a Field for the worker hierarchy path.
func HierarchyPath(path string) Field {
	return Field{Key: "hierarchy_path", Value: path}
}

// WorkerID creates a Field for the worker identifier.
func WorkerID(id string) Field {
	return Field{Key: "worker_id", Value: id}
}

// WorkerType creates a Field for the worker type.
func WorkerType(wt string) Field {
	return Field{Key: "worker_type", Value: wt}
}

// ActionName creates a Field for the action name.
func ActionName(name string) Field {
	return Field{Key: "action_name", Value: name}
}

// CorrelationID creates a Field for request correlation.
func CorrelationID(id string) Field {
	return Field{Key: "correlation_id", Value: id}
}

// DurationMs creates a Field for duration in milliseconds.
func DurationMs(ms int64) Field {
	return Field{Key: "duration_ms", Value: ms}
}

// Attempts creates a Field for retry attempt count.
func Attempts(n int) Field {
	return Field{Key: "attempts", Value: n}
}

// Capacity creates a Field for queue/buffer capacity.
func Capacity(n int) Field {
	return Field{Key: "capacity", Value: n}
}

// Length creates a Field for queue/buffer length.
func Length(n int) Field {
	return Field{Key: "length", Value: n}
}

// Reason creates a Field for explaining why something happened.
func Reason(r string) Field {
	return Field{Key: "reason", Value: r}
}
