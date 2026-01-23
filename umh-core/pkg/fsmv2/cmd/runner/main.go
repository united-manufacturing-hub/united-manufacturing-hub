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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

func main() {
	// Command-line flags
	var (
		scenarioName = flag.String("scenario", "simple", "scenario name from registry")
		duration     = flag.Duration("duration", 0, "run duration, 0 means endless until Ctrl+C")
		logLevel     = flag.String("log-level", "info", "debug, info, warn, error")
		tickInterval = flag.Duration("tick", 100*time.Millisecond, "tick interval")
		listFlag     = flag.Bool("list", false, "list available scenarios and exit")
		traceFlag    = flag.Bool("trace", false, "enable trace logging")
		dumpStore    = flag.Bool("dump-store", false, "dump store deltas and final state after scenario")
	)

	flag.Parse()

	// List scenarios if requested
	if *listFlag {
		fmt.Println("Available scenarios:")

		for name, scenario := range examples.Registry {
			fmt.Printf("  %-15s - %s\n", name, scenario.Description)
		}

		return
	}

	// Setup logger with human-readable console output
	level, err := parseLogLevel(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %v\n", err)
		os.Exit(1)
	}

	if *traceFlag {
		level = zap.DebugLevel
	}

	// Create a human-readable console encoder
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "",                          // Disable caller for cleaner output
		FunctionKey:    zapcore.OmitKey,             // Disable function name
		MessageKey:     "msg",
		StacktraceKey:  "",                          // Disable stacktrace for cleaner output
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // Colored levels
		EncodeTime:     zapcore.TimeEncoderOfLayout("15:04:05.000"), // Time with milliseconds
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)

	logger := zap.New(core)

	defer func() { _ = logger.Sync() }()

	// Look up scenario from registry
	scenario, exists := examples.Registry[*scenarioName]
	if !exists {
		logger.Fatal("Scenario not found",
			zap.String("scenario", *scenarioName),
			zap.String("hint", "Use --list to see available scenarios"),
		)
	}

	// Setup context (no cancellation from signal - shutdown is handled via Shutdown())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wrap context with timeout if duration specified
	if *duration > 0 {
		var timeoutCancel context.CancelFunc

		ctx, timeoutCancel = context.WithTimeout(ctx, *duration)
		defer timeoutCancel()
	}

	// Setup store
	store := examples.SetupStore(logger.Sugar())

	// Print scenario info
	durationStr := "endless (until Ctrl+C)"
	if *duration > 0 {
		durationStr = duration.String()
	}

	logger.Info("Starting scenario",
		zap.String("name", *scenarioName),
		zap.String("description", scenario.Description),
		zap.String("duration", durationStr),
		zap.Duration("tick", *tickInterval),
	)

	// Run the scenario
	result, err := examples.Run(ctx, examples.RunConfig{
		Scenario:           scenario,
		TickInterval:       *tickInterval,
		Logger:             logger.Sugar(),
		Store:              store,
		EnableTraceLogging: *traceFlag,
		DumpStore:          *dumpStore,
	})
	if err != nil {
		logger.Fatal("Failed to start scenario", zap.Error(err))
	}

	// Setup signal handling - call graceful shutdown instead of cancelling context
	// First signal: graceful shutdown
	// Second signal: force immediate exit
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("Received signal, initiating graceful shutdown...",
			zap.String("signal", sig.String()),
			zap.String("hint", "Press Ctrl+C again to force immediate exit"))

		// Start shutdown in background so we can still listen for second signal
		go result.Shutdown()

		// Wait for either second signal (force exit) or graceful completion
		select {
		case sig := <-sigChan:
			logger.Warn("Received second signal, forcing immediate exit!",
				zap.String("signal", sig.String()))
			os.Exit(1)
		case <-result.Done:
			// Graceful shutdown completed before second signal
		}
	}()

	// Wait for completion
	<-result.Done

	// Print completion message
	logger.Info("Scenario completed",
		zap.String("name", *scenarioName),
	)
}

// parseLogLevel converts string log level to zap level using zap's built-in parser.
func parseLogLevel(level string) (zapcore.Level, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return zap.InfoLevel, fmt.Errorf("unknown log level: %s", level)
	}

	return lvl, nil
}
