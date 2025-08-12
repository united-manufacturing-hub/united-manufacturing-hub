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

package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	httpv2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"go.uber.org/zap"
)

// CheckIfAPIIsReachable checks if the management.umh.app/api endpoint is reachable.
func CheckIfAPIIsReachable(insecureTLS bool, apiURL string, logger *zap.SugaredLogger) bool {
	baseUrl := apiURL

	// Log proxy configuration that ProxyFromEnvironment will use
	logger.Debugf("Proxy configuration for API reachability check:")

	proxyVars := []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"}
	for _, envVar := range proxyVars {
		if value := os.Getenv(envVar); value != "" {
			logger.Debugf("  %s = %s", envVar, value)
		} else {
			logger.Debugf("  %s = (not set)", envVar)
		}
	}

	if insecureTLS {
		logger.Debugf("Insecure TLS is enabled, skipping certificate verification")
	}

	client := httpv2.GetClient(insecureTLS)

	// Create a context with timeout for the reachability check
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseUrl, nil)
	if err != nil {
		logger.Errorf("Error creating request for API reachability check: %v", err)
		return false
	}

	response, err := client.Do(req)
	if err != nil {
		logger.Errorf("Error while checking if API is reachable: %v", err)

		return false
	}

	if response == nil {
		logger.Errorf("Received nil response from API check")

		return false
	}

	defer func() {
		err := response.Body.Close()
		if err != nil {
			logger.Errorf("Error while closing response body: %v", err)
		}
	}()
	// Check if response is 200 OK
	if response.StatusCode != http.StatusOK {
		logger.Errorf("API check response code is not 200 OK: %v", response.StatusCode)

		return false
	}

	if strings.HasPrefix(baseUrl, "https://") {
		if response.TLS == nil {
			logger.Errorf("API check got HTTP response for an HTTPS endpoint")

			return false
		}

		for i, cert := range response.TLS.PeerCertificates {
			logger.Debugf("Certificate %d:", i)
			logger.Debugf("    Subject: %s", cert.Subject)
			logger.Debugf("    Issuer: %s", cert.Issuer)
			logger.Debugf("    Serial Number: %s", cert.SerialNumber)
			logger.Debugf("    IsCA: %v", cert.IsCA)
			logger.Debugf("    DNSNames: %v", cert.DNSNames)
			logger.Debugf("    NotBefore: %v", cert.NotBefore)
			logger.Debugf("    NotAfter: %v", cert.NotAfter)
		}
	} else {
		logger.Debugf("API is reachable over HTTP, no certificate verification performed")
	}

	return true
}
