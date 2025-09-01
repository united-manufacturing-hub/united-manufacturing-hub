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
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogLevel represents the logging level.
type LogLevel string

// LogFormat represents the logging format.
type LogFormat string

const (
	// DebugLevel logs debug level messages.
	DebugLevel LogLevel = "DEBUG"
	// InfoLevel logs informational messages.
	InfoLevel LogLevel = "INFO"
	// WarnLevel logs warning messages.
	WarnLevel LogLevel = "WARN"
	// ErrorLevel logs error messages.
	ErrorLevel LogLevel = "ERROR"
	// DPanicLevel logs critical errors and panics in development.
	DPanicLevel LogLevel = "DPANIC"
	// PanicLevel logs critical errors and panics.
	PanicLevel LogLevel = "PANIC"
	// FatalLevel logs fatal errors.
	FatalLevel LogLevel = "FATAL"
	// ProductionLevel is an alias for InfoLevel, used for easier configuration.
	ProductionLevel LogLevel = "PRODUCTION"

	// FormatConsole indicates human-readable console format.
	FormatConsole LogFormat = "CONSOLE"
	// FormatJSON indicates structured JSON format.
	FormatJSON LogFormat = "JSON"
	// FormatPretty indicates highly human-readable format.
	FormatPretty LogFormat = "PRETTY"
)

var (
	// Mutex to ensure thread safety for logger initialization.
	loggerMutex sync.Once
	// initialized tracks whether the global logger has been initialized.
	initialized bool
)

// getLogLevel converts a string log level to zapcore.Level.
func getLogLevel(level LogLevel) zapcore.Level {
	switch strings.ToUpper(string(level)) {
	case string(DebugLevel):
		return zapcore.DebugLevel
	case string(InfoLevel):
		return zapcore.InfoLevel
	case string(WarnLevel):
		return zapcore.WarnLevel
	case string(ErrorLevel):
		return zapcore.ErrorLevel
	case string(DPanicLevel):
		return zapcore.DPanicLevel
	case string(PanicLevel):
		return zapcore.PanicLevel
	case string(FatalLevel):
		return zapcore.FatalLevel
	case string(ProductionLevel):
		return zapcore.InfoLevel
	default:
		return zapcore.InfoLevel
	}
}

// getLogFormat returns the log format based on the environment variable or default value.
func getLogFormat(defaultFormat LogFormat) LogFormat {
	format := LogFormat(getEnv("LOGGING_FORMAT", string(defaultFormat)))
	if format != FormatConsole && format != FormatJSON && format != FormatPretty {
		return defaultFormat
	}

	return format
}

// getEnv gets environment variable with a default value.
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	return value
}

// timeEncoder encodes the time as a human-readable timestamp.
func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05 MST"))
}

// New creates a new zap logger with the specified log level and format.
func New(logLevel string, logFormat LogFormat) *zap.Logger {
	level := getLogLevel(LogLevel(logLevel))

	// Configure encoder
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "component",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Configure different encode levels and time formats based on the log format
	if logFormat == FormatConsole || logFormat == FormatPretty {
		// For console format, use colors and custom time format
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoderConfig.EncodeTime = timeEncoder
		// Define a custom console encoder format
		encoderConfig.ConsoleSeparator = " | "
	} else {
		// For JSON format, use standard encoders
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// Choose the encoder based on format
	var encoder zapcore.Encoder

	switch logFormat {
	case FormatPretty:
		encoder = NewPrettyConsoleEncoder(encoderConfig)
	case FormatConsole:
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	case FormatJSON:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	default:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Core configuration
	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		zap.NewAtomicLevelAt(level),
	)

	// Create the logger
	logger := zap.New(core, zap.AddCaller())

	return logger
}

// Initialize sets up the global logger with the specified log level using zap.ReplaceGlobals().
func Initialize() {
	loggerMutex.Do(func() {
		logLevel := getEnv("LOGGING_LEVEL", string(ProductionLevel))
		logFormat := getLogFormat(FormatPretty) // Default to human-readable pretty format
		logger := New(logLevel, logFormat)

		// Log initialization information
		logger.Info("Logger initialized",
			zap.String("level", logLevel),
			zap.String("format", string(logFormat)))

		// Replace the global loggers with our configured logger
		zap.ReplaceGlobals(logger)

		initialized = true
	})
}

// GetLogger returns the global logger, initializing it if needed.
func GetLogger() *zap.Logger {
	if !initialized {
		Initialize()
	}

	return zap.L()
}

// GetSugaredLogger returns the global sugared logger, initializing it if needed.
func GetSugaredLogger() *zap.SugaredLogger {
	if !initialized {
		Initialize()
	}

	return zap.S()
}

// Sync flushes any buffered log entries.
func Sync() error {
	return zap.L().Sync()
}

// For creates a named logger for a specific component.
func For(component string) *zap.SugaredLogger {
	if !initialized {
		Initialize()
	}

	return zap.S().Named(component)
}
