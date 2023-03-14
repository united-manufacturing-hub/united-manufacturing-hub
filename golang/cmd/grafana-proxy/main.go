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

var buildtime string

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)
	zap.S().Infof("This is grafana-proxy build date: %s", buildtime)

	internal.Initfgtrace()
	var FactoryInpitAPIKeyEnvSet bool
	FactoryInputAPIKey, FactoryInpitAPIKeyEnvSet = os.LookupEnv("FACTORYINPUT_KEY")
	if !FactoryInpitAPIKeyEnvSet {
		zap.S().Fatal("Factory Input API Key (FACTORYINPUT_KEY) must be set")
	}
	var FactoryInputUserEnvSet bool
	FactoryInputUser, FactoryInputUserEnvSet = os.LookupEnv("FACTORYINPUT_USER")
	if !FactoryInputUserEnvSet {
		zap.S().Fatal("Factory Input User (FACTORYINPUT_USER) must be set")
	}
	var FactoryInputBaseURLEnvSet bool
	FactoryInputBaseURL, FactoryInputBaseURLEnvSet = os.LookupEnv("FACTORYINPUT_BASE_URL")
	if !FactoryInputBaseURLEnvSet {
		zap.S().Fatal("Factory Input Base URL (FACTORYINPUT_BASE_URL) must be set")
	}
	var FactoryInsightBaseUrlEnvSet bool
	FactoryInsightBaseUrl, FactoryInsightBaseUrlEnvSet = os.LookupEnv("FACTORYINSIGHT_BASE_URL")
	if !FactoryInsightBaseUrlEnvSet {
		zap.S().Fatal("Factory Insight Base Url (FACTORYINSIGHT_BASE_URL) must be set")
	}

	if len(FactoryInputAPIKey) == 0 {
		zap.S().Error("Factoryinput API Key not set")
		return
	}
	if FactoryInputUser == "" {
		zap.S().Error("Factoryinput user not set")
		return
	}
	if FactoryInputBaseURL == "" {
		zap.S().Error("Factoryinput base url not set")
		return
	}
	if FactoryInsightBaseUrl == "" {
		zap.S().Error("Factoryinsight base url not set")
		return
	}

	if !strings.HasSuffix(FactoryInputBaseURL, "/") {
		FactoryInputBaseURL = fmt.Sprintf("%s/", FactoryInputBaseURL)
	}
	if !strings.HasSuffix(FactoryInsightBaseUrl, "/") {
		FactoryInsightBaseUrl = fmt.Sprintf("%s/", FactoryInsightBaseUrl)
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

	SetupRestAPI()
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
