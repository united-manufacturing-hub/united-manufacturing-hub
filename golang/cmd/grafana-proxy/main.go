// Copyright 2023 UMH Systems GmbH
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
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var shutdownEnabled bool

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	factoryInputAPIKey, err := env.GetAsString("FACTORYINPUT_KEY", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	factoryInputUser, err := env.GetAsString("FACTORYINPUT_USER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	factoryInputBaseURL, err := env.GetAsString("FACTORYINPUT_BASE_URL", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	factoryInsightBaseUrl, err := env.GetAsString("FACTORYINSIGHT_BASE_URL", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	if !strings.HasSuffix(factoryInputBaseURL, "/") {
		FactoryInputBaseURL = fmt.Sprintf("%s/", factoryInputBaseURL)
	}
	if !strings.HasSuffix(factoryInsightBaseUrl, "/") {
		factoryInsightBaseUrl = fmt.Sprintf("%s/", factoryInsightBaseUrl)
	}

	health := healthcheck.NewHandler()
	shutdownEnabled = false
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))
	health.AddReadinessCheck("shutdownEnabled", isShutdownEnabled())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Fatalf("Failed to start healthcheck: %s", err)
		}
	}()

	SetupRestAPI(factoryInputBaseURL, factoryInputUser, factoryInputAPIKey, factoryInsightBaseUrl)
	zap.S().Infof("Ready to proxy connections")

	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	go func() {
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.
		//
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Received SIGTERM", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {} // block forever
}

func isShutdownEnabled() healthcheck.Check {
	return func() error {
		if shutdownEnabled {
			return fmt.Errorf("shutdown")
		}
		return nil
	}
}

func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	shutdownEnabled = true

	zap.S().Infof("Successful shutdown. Exiting.")
	os.Exit(0)
}
