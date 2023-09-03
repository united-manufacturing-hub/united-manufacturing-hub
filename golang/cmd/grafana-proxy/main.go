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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(log *zap.SugaredLogger) {
		err := logger.Sync(log)
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	rs := &restServers{}
	gs := internal.NewGracefulShutdown(rs.onShutdown)
	defer gs.Wait()

	f, err := initFactory()
	if err != nil {
		zap.S().Fatalf("Failed to initialize factory: %s", err)
	}

	rs.factorySrv = f.factoryRestServer()
	rs.healthSrv = healthcheckRestServer(gs)
	rs.run()
	zap.S().Infof("ready to proxy connections")
}

type restServers struct {
	factorySrv *http.Server // Main REST API server
	healthSrv  *http.Server // Healthcheck server
}

// Start the factory and healthcheck servers in their own goroutines.
func (rs *restServers) run() {
	go func() {
		/* #nosec G114 */
		err := rs.healthSrv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			zap.S().Fatalf("failed to start healthcheck server: %s", err)
		}
	}()
	go func() {
		err := rs.factorySrv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			zap.S().Fatalf("failed to start factory server: %s", err)
		}
	}()
}

// Shutdown the factory and healthcheck servers within a 30 second timeout.
func (rs *restServers) onShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	// Servers shouldn't be nil, but better safe than sorry.
	if rs.factorySrv != nil {
		fErr := rs.factorySrv.Shutdown(ctx)
		if fErr != nil {
			err = fmt.Errorf("failed to shutdown factory server: %w", fErr)
		} else {
			zap.S().Info("factory http server successfully shut down")
		}
	}
	if rs.healthSrv != nil {
		hErr := rs.healthSrv.Shutdown(ctx)
		if hErr != nil {
			// Append to existing error if there is one.
			errors.Join(err, fmt.Errorf("failed to shutdown healthcheck server: %w", hErr))
		} else {
			zap.S().Info("healthcheck http server successfully shut down")
		}
	}
	return err
}

type Factory struct {
	APIKey         string // FACTORYINPUT_KEY
	User           string // FACTORYINPUT_USER
	BaseURL        string // FACTORYINPUT_BASE_URL
	InsightBaseURL string // FACTORYINSIGHT_BASE_URL
}

// Initialize the factory struct by reading the required environment variables set in factoryEnvVars.
// Returns an error if any of the environment variables are missing.
func initFactory() (Factory, error) {
	var f Factory
	formatUrl := func(url string) string {
		if !strings.HasSuffix(url, "/") {
			url = fmt.Sprintf("%s/", url)
		}
		return url
	}

	factoryEnvVars := []string{"FACTORYINPUT_KEY", "FACTORYINPUT_USER", "FACTORYINPUT_BASE_URL", "FACTORYINSIGHT_BASE_URL"}
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
			f.BaseURL = formatUrl(val)
		case "FACTORYINSIGHT_BASE_URL":
			f.InsightBaseURL = formatUrl(val)
		}
	}
	return f, nil
}
