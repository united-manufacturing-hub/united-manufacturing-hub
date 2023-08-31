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
	"net/http"
	"strings"

	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

var factoryEnvVars = []string{"FACTORYINPUT_KEY", "FACTORYINPUT_USER", "FACTORYINPUT_BASE_URL", "FACTORYINSIGHT_BASE_URL"}

type Factory struct {
	APIKey         string
	User           string
	BaseURL        string
	InsightBaseURL string
}

// Initialize the factory struct by reading the required environment variables set in factoryEnvVars.
// Returns an error if any of the environment variables are missing.
func initFactory() (Factory, error) {
	var f Factory
	for _, envVar := range factoryEnvVars {
		val, err := env.GetAsString(envVar, true, "")
		if err != nil {
			return f, err
		}

		switch envVar {
		case "FACTORYINPUT_KEY":
			f.APIKey = val
		case "FACTORYINPUT_USER":
			f.User = val
		case "FACTORYINPUT_BASE_URL":
			if !strings.HasSuffix(val, "/") {
				val = fmt.Sprintf("%s/", val)
			}
			f.BaseURL = val
		case "FACTORYINSIGHT_BASE_URL":
			if !strings.HasSuffix(val, "/") {
				val = fmt.Sprintf("%s/", val)
			}
			f.InsightBaseURL = val
		}
	}
	return f, nil
}

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
	gracefulShutdown := internal.NewGracefulShutdown(nil)

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))
	health.AddReadinessCheck("shutdownEnabled", func() error {
		if gracefulShutdown.ShuttingDown() {
			return fmt.Errorf("shutdown")
		}
		return nil
	})

	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Fatalf("Failed to start healthcheck: %s", err)
		}
	}()

	f, err := initFactory()
	if err != nil {
		zap.S().Fatalf("Failed to initialize factory: %s", err)
	}

	f.setupRestAPI()
	zap.S().Infof("Ready to proxy connections")

	select {} // block forever
}
