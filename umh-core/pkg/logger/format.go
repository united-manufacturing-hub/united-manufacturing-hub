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

package logger

import (
	"fmt"
	"time"

	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// PrettyConsoleEncoder is a custom encoder that produces human-readable logs
// in a format like:
// [2006-01-02 15:04:05] [INFO] [ComponentName] Message here
type PrettyConsoleEncoder struct {
	*zapcore.EncoderConfig
	pool buffer.Pool
}

// NewPrettyConsoleEncoder creates a new PrettyConsoleEncoder instance.
func NewPrettyConsoleEncoder(cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &PrettyConsoleEncoder{
		EncoderConfig: &cfg,
		pool:          buffer.NewPool(),
	}
}

// Clone implements zapcore.Encoder interface
func (e *PrettyConsoleEncoder) Clone() zapcore.Encoder {
	return &PrettyConsoleEncoder{
		EncoderConfig: e.EncoderConfig,
		pool:          e.pool,
	}
}

// EncodeEntry formats a log entry in a human-readable format.
func (e *PrettyConsoleEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	line := e.pool.Get()

	// not needed anymore as S6 automatically adds timestamps
	// Format timestamp
	// line.AppendByte('[')
	// if entry.Time.IsZero() {
	// 	line.AppendString("no timestamp")
	// } else {
	//line.AppendString(entry.Time.Format("2006-01-02 15:04:05 MST"))
	// }
	//line.AppendByte(']')

	// Format log level with padding for alignment
	line.AppendString(" [")
	level := entry.Level.CapitalString()
	line.AppendString(level)

	line.AppendByte(']')
	// Add tab after level
	line.AppendByte('\t')

	// Format caller information if available
	if entry.Caller.Defined {
		line.AppendString("[")
		line.AppendString(entry.Caller.TrimmedPath())
		line.AppendString(":")
		line.AppendString(fmt.Sprint(entry.Caller.Line))
		line.AppendByte(']')
		line.AppendByte('\t')
	}

	// Format component name if available
	if entry.LoggerName != "" {
		line.AppendString("[")
		line.AppendString(entry.LoggerName)
		line.AppendByte(']')
		line.AppendByte('\t')
		line.AppendByte('\t')
		line.AppendByte('\t')
	}

	// Format log message
	line.AppendString(entry.Message)

	// Add fields if any
	if len(fields) > 0 {
		line.AppendString(" - ")
		addFields(line, fields)
	}

	// Add line ending
	line.AppendString(e.LineEnding)

	return line, nil
}

// Adds fields to the log line
func addFields(line *buffer.Buffer, fields []zapcore.Field) {
	enc := zapcore.NewMapObjectEncoder()
	for i, field := range fields {
		field.AddTo(enc)
		if i > 0 {
			line.AppendString(", ")
		}
		line.AppendString(field.Key)
		line.AppendString("=")
		line.AppendString(fmt.Sprintf("%v", enc.Fields[field.Key]))
	}
}

// Compatible encoder methods we need to implement

// AddArray implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddArray(key string, arr zapcore.ArrayMarshaler) error {
	return zapcore.NewConsoleEncoder(*e.EncoderConfig).AddArray(key, arr)
}

// AddObject implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddObject(key string, obj zapcore.ObjectMarshaler) error {
	return zapcore.NewConsoleEncoder(*e.EncoderConfig).AddObject(key, obj)
}

// AddBinary implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddBinary(key string, value []byte) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddBinary(key, value)
}

// AddByteString implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddByteString(key string, value []byte) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddByteString(key, value)
}

// AddBool implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddBool(key string, value bool) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddBool(key, value)
}

// AddComplex128 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddComplex128(key string, value complex128) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddComplex128(key, value)
}

// AddComplex64 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddComplex64(key string, value complex64) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddComplex64(key, value)
}

// AddDuration implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddDuration(key string, value time.Duration) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddDuration(key, value)
}

// AddFloat64 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddFloat64(key string, value float64) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddFloat64(key, value)
}

// AddFloat32 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddFloat32(key string, value float32) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddFloat32(key, value)
}

// AddInt implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddInt(key string, value int) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddInt(key, value)
}

// AddInt64 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddInt64(key string, value int64) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddInt64(key, value)
}

// AddInt32 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddInt32(key string, value int32) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddInt32(key, value)
}

// AddInt16 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddInt16(key string, value int16) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddInt16(key, value)
}

// AddInt8 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddInt8(key string, value int8) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddInt8(key, value)
}

// AddString implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddString(key string, value string) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddString(key, value)
}

// AddTime implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddTime(key string, value time.Time) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddTime(key, value)
}

// AddUint implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddUint(key string, value uint) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddUint(key, value)
}

// AddUint64 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddUint64(key string, value uint64) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddUint64(key, value)
}

// AddUint32 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddUint32(key string, value uint32) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddUint32(key, value)
}

// AddUint16 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddUint16(key string, value uint16) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddUint16(key, value)
}

// AddUint8 implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddUint8(key string, value uint8) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddUint8(key, value)
}

// AddUintptr implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddUintptr(key string, value uintptr) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).AddUintptr(key, value)
}

// AddReflected implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) AddReflected(key string, value interface{}) error {
	return zapcore.NewConsoleEncoder(*e.EncoderConfig).AddReflected(key, value)
}

// OpenNamespace implements zapcore.ObjectEncoder
func (e *PrettyConsoleEncoder) OpenNamespace(key string) {
	zapcore.NewConsoleEncoder(*e.EncoderConfig).OpenNamespace(key)
}
