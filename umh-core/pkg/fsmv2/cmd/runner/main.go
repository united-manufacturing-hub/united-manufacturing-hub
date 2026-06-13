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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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

	// Validate tick interval and duration to prevent runtime panics
	if *tickInterval <= 0 {
		fmt.Fprintf(os.Stderr, "Invalid tick interval: must be positive, got %v\n", *tickInterval)
		os.Exit(1)
	}

	if *duration < 0 {
		fmt.Fprintf(os.Stderr, "Invalid duration: must be non-negative, got %v\n", *duration)
		os.Exit(1)
	}

	if *listFlag {
		fmt.Println("Available scenarios:")

		scenarios := examples.ListScenarios()

		names := make([]string, 0, len(scenarios))
		for name := range scenarios {
			names = append(names, name)
		}

		sort.Strings(names)

		for _, name := range names {
			fmt.Printf("  %-15s - %s\n", name, scenarios[name])
		}

		return
	}

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %v\n", err)
		os.Exit(1)
	}

	if *traceFlag {
		level = zap.DebugLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "",              // Disable caller for cleaner output
		FunctionKey:    zapcore.OmitKey, // Disable function name
		MessageKey:     "msg",
		StacktraceKey:  "", // Disable stacktrace for cleaner output
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,            // Colored levels
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

	v1Scenario, isV1 := examples.Registry[*scenarioName]
	v2Scenario, isV2 := examples.RegistryV2[*scenarioName]

	if !isV1 && !isV2 {
		logger.Fatal("Scenario not found",
			zap.String("scenario", *scenarioName),
			zap.String("hint", "Use --list to see available scenarios"),
		)
	}

	description := v1Scenario.Description
	if isV2 {
		description = v2Scenario.Description
	}

	// One signal owner: the CLI creates the cancellable ctx and is the only
	// signal.Notify site, installed BEFORE examples.Run so a SIGINT during a
	// running driver cannot kill the process without teardown. The first
	// SIGINT cancels the ctx (a v2 driver sees it, the runner tears down
	// gracefully); a second SIGINT force-exits.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDuration, applyCtxTimeout := routeDuration(isV2, *duration)
	if applyCtxTimeout {
		var timeoutCancel context.CancelFunc

		ctx, timeoutCancel = context.WithTimeout(ctx, *duration)
		defer timeoutCancel()
	}

	// runDone closes once main returns, i.e. once the run has fully torn down.
	// The signal owner waits on it so a second SIGINT can still force-exit
	// while teardown is in flight; cancelling the ctx (the first SIGINT) does
	// not close runDone.
	runDone := make(chan struct{})
	defer close(runDone)

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go handleSignals(
		sigChan,
		runDone,
		func() {
			logger.Info("Received signal, initiating graceful shutdown...",
				zap.String("hint", "Press Ctrl+C again to force immediate exit"))
			cancel()
		},
		func() {
			logger.Sugar().Warnw("forced_exit_second_signal",
				"reason", "received_second_signal")
			os.Exit(1)
		},
	)

	store := examples.SetupStore(deps.NewFSMLogger(logger.Sugar()))

	durationStr := "endless (until Ctrl+C)"
	if *duration > 0 {
		durationStr = duration.String()
	}

	logger.Info("Starting scenario",
		zap.String("name", *scenarioName),
		zap.String("description", description),
		zap.String("duration", durationStr),
		zap.Duration("tick", *tickInterval),
	)

	result, err := examples.Run(ctx, examples.RunConfig{
		Scenario:           v1Scenario,
		ScenarioV2:         v2Scenario,
		Duration:           runDuration,
		TickInterval:       *tickInterval,
		Logger:             deps.NewFSMLogger(logger.Sugar()),
		Store:              store,
		EnableTraceLogging: *traceFlag,
		DumpStore:          *dumpStore,
	})
	if err != nil {
		if isCleanInterruptExit(err, ctx.Err()) {
			logger.Info("Scenario interrupted",
				zap.String("name", *scenarioName),
			)

			return
		}

		logger.Fatal("Failed to start scenario", zap.Error(err))
	}

	<-result.Done

	logger.Info("Scenario completed",
		zap.String("name", *scenarioName),
	)
}

// routeDuration decides how a --duration flag binds to a run. A v2 scenario
// treats the duration as a post-driver settle window, so it flows into
// RunConfig.Duration and never bounds the run with a ctx timeout. A v1 scenario
// has no settle window, so the duration bounds the whole run via a ctx timeout.
// A zero duration stays endless on both paths.
func routeDuration(isV2 bool, duration time.Duration) (runDuration time.Duration, applyCtxTimeout bool) {
	if duration <= 0 {
		return 0, false
	}

	if isV2 {
		return duration, false
	}

	return 0, true
}

// isCleanInterruptExit reports whether a run error is an interrupt-induced
// context cancellation rather than a genuine startup failure. An interrupt
// cancels the context and surfaces as a runErr that wraps ctxErr; that is a
// clean exit, not a fatal exit-1.
func isCleanInterruptExit(runErr error, ctxErr error) bool {
	return ctxErr != nil && errors.Is(runErr, ctxErr)
}

// handleSignals tears down on the first signal and force-exits on the second.
// The first signal calls onFirstSignal to start graceful teardown; a second
// signal calls forceExit instead of waiting for done to close.
func handleSignals(sigCh <-chan os.Signal, done <-chan struct{}, onFirstSignal func(), forceExit func()) {
	<-sigCh
	onFirstSignal()

	select {
	case <-sigCh:
		forceExit()
	case <-done:
	}
}

// parseLogLevel converts string log level to zap level using zap's built-in parser.
func parseLogLevel(level string) (zapcore.Level, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return zap.InfoLevel, fmt.Errorf("unknown log level: %s", level)
	}

	return lvl, nil
}
