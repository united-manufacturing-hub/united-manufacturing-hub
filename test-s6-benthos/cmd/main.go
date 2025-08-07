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
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"test-s6-benthos/internal/s6manager"
	"test-s6-benthos/internal/testservice"

	"go.uber.org/zap"
)

func main() {
	// Set up logging
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	sugar := logger.Sugar()
	sugar.Info("Starting s6-benthos integration test")

	// Create context that cancels on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		sugar.Infof("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Create filesystem service and s6 service
	fsService := testservice.NewFilesystemService()
	s6Service := testservice.NewS6Service(sugar)

	// Create s6 manager
	manager := s6manager.NewManager(s6Service, fsService, sugar)

	// Create a monitoring goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				manager.LogServiceStatus(ctx)
			}
		}
	}()

	// Test configuration
	const numBenthosInstances = 490
	const testDuration = 2 * time.Minute

	sugar.Infof("Creating %d benthos instances for %v", numBenthosInstances, testDuration)

	// Create multiple benthos instances sequentially
	var errors []error

	for i := 0; i < numBenthosInstances; i++ {
		serviceName := fmt.Sprintf("benthos-test-%d", i)
		sugar.Infof("Starting benthos instance %s", serviceName)

		if err := manager.RunBenthos(ctx, serviceName, i+8080); err != nil {
			sugar.Errorf("Failed to run benthos instance %s: %v", serviceName, err)
			errors = append(errors, fmt.Errorf("benthos-%d failed: %w", i, err))
			continue
		}

		sugar.Infof("Benthos instance %s completed successfully", serviceName)
	}

	sugar.Info("All benthos instances completed")

	// Check for errors
	errorCount := len(errors)
	for _, err := range errors {
		sugar.Error(err)
	}

	if errorCount > 0 {
		sugar.Errorf("Test completed with %d errors", errorCount)
		os.Exit(1)
	}

	sugar.Info("Test completed successfully!")
}
