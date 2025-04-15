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
	"crypto/tls"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// CheckIfAPIIsReachable checks if the management.umh.app/api endpoint is reachable
func CheckIfAPIIsReachable(insecureTLS bool, apiURL string, logger *zap.SugaredLogger) bool {
	baseUrl := apiURL

	// Copy the default transport to avoid modifying it (and then modify the copy)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecureTLS}
	client := &http.Client{Transport: tr}

	response, err := client.Get(baseUrl)

	if err != nil {
		logger.Errorf("Error while checking if API is reachable: %v", err)
		return false
	}
	defer response.Body.Close()
	// Check if response is 200 OK
	if response.StatusCode != 200 {
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
