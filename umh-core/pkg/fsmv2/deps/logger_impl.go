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

import (
	"io"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogLevel controls the minimum severity for log output.
type LogLevel int8

const (
	LevelDebug LogLevel = iota - 1
	LevelInfo
	LevelWarn
	LevelError
)

// zapLogger implements FSMLogger by wrapping zap.SugaredLogger.
type zapLogger struct {
	sugar      *zap.SugaredLogger
	baseFields []Field
}

// NewFSMLogger creates a new FSMLogger wrapping a zap.SugaredLogger.
func NewFSMLogger(sugar *zap.SugaredLogger) FSMLogger {
	if sugar == nil {
		panic("NewFSMLogger: sugar cannot be nil")
	}

	return &zapLogger{sugar: sugar}
}

// Debug logs at DEBUG level with structured fields.
func (l *zapLogger) Debug(msg string, fields ...Field) {
	l.sugar.Debugw(msg, fieldsToArgs(l.baseFields, fields)...)
}

// Info logs at INFO level with structured fields.
func (l *zapLogger) Info(msg string, fields ...Field) {
	l.sugar.Infow(msg, fieldsToArgs(l.baseFields, fields)...)
}

// Warn logs at WARN level with structured fields.
func (l *zapLogger) Warn(msg string, fields ...Field) {
	l.sugar.Warnw(msg, fieldsToArgs(l.baseFields, fields)...)
}

// SentryWarn logs at WARN level with required Feature for Sentry routing.
func (l *zapLogger) SentryWarn(feature Feature, msg string, fields ...Field) {
	// Prepend feature to fields for Sentry hook extraction
	allFields := append([]Field{{Key: "feature", Value: string(feature)}}, fields...)
	l.sugar.Warnw(msg, fieldsToArgs(l.baseFields, allFields)...)
}

// SentryError logs at ERROR level with required Feature and error for Sentry.
func (l *zapLogger) SentryError(feature Feature, err error, msg string, fields ...Field) {
	// Prepend feature and error to fields for Sentry hook extraction
	allFields := append([]Field{
		{Key: "feature", Value: string(feature)},
		{Key: "error", Value: err},
	}, fields...)
	l.sugar.Errorw(msg, fieldsToArgs(l.baseFields, allFields)...)
}

// With returns a new FSMLogger with additional context fields.
func (l *zapLogger) With(fields ...Field) FSMLogger {
	newBaseFields := make([]Field, len(l.baseFields)+len(fields))
	copy(newBaseFields, l.baseFields)
	copy(newBaseFields[len(l.baseFields):], fields)

	return &zapLogger{
		sugar:      l.sugar,
		baseFields: newBaseFields,
	}
}

// NewJSONFSMLogger creates an FSMLogger that writes JSON to the provided writer.
// Use this in tests to capture and verify log output without importing zap.
func NewJSONFSMLogger(w io.Writer, level LogLevel) FSMLogger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(w),
		zapcore.Level(level),
	)

	return NewFSMLogger(zap.New(core).Sugar())
}

// fieldsToArgs converts Fields to zap's variadic key-value args.
func fieldsToArgs(baseFields, additionalFields []Field) []any {
	totalLen := len(baseFields) + len(additionalFields)
	if totalLen == 0 {
		return nil
	}

	args := make([]any, 0, totalLen*2)

	for _, f := range baseFields {
		args = append(args, f.Key, f.Value)
	}

	for _, f := range additionalFields {
		args = append(args, f.Key, f.Value)
	}

	return args
}
