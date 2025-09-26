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

package http_test

import (
	"context"
	"fmt"
	netHTTP "net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

var _ = Describe("Requester", func() {

	var log *zap.SugaredLogger

	const apiUrl = "https://management.umh.app/api"

	BeforeEach(func() {
		log = logger.For("RequesterTest")
	})

	// doHTTPRequestWithRetry is a test helper that retries HTTP requests on connection issues
	doHTTPRequestWithRetry := func(ctx context.Context, url string, header map[string]string, cookies *map[string]string, insecureTLS bool, logger *zap.SugaredLogger) (*netHTTP.Response, error) {
		var lastErr error
		for i := range 10 {
			response, err := http.DoHTTPRequest(ctx, url, header, cookies, insecureTLS, logger)
			if err == nil {
				return response, nil
			}

			// Only retry on connection-related errors
			if strings.Contains(err.Error(), "connection") ||
				strings.Contains(err.Error(), "timeout") ||
				strings.Contains(err.Error(), "EOF") {
				lastErr = err
				time.Sleep(time.Second * time.Duration(i+1)) // Exponential backoff

				continue
			}

			// For non-connection errors, return immediately
			return response, err
		}

		return nil, fmt.Errorf("failed after 10 retries, last error: %w", lastErr)
	}

	// The tests in this context are ported from the old requester_test.go file
	Context("Requester", func() {
		var header map[string]string
		var cookies map[string]string
		var data map[string]string

		BeforeEach(func() {
			header = map[string]string{
				"Content-Type": "application/json",
			}
			cookies = map[string]string{
				"test": "test",
			}
			data = map[string]string{
				"test": "test",
			}
		})

		Context("GetRequest", func() {
			It("should return no error", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				_, status, err := http.GetRequest[any](ctx, http.LoginEndpoint, header, &cookies, false, apiUrl, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(200))
			})
		})

		Context("PostRequest", func() {
			It("should return error an error", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				_, status, err := http.PostRequest[any](ctx, http.LoginEndpoint, &data, header, &cookies, false, apiUrl, log)
				Expect(err).To(HaveOccurred())
				Expect(status).To(Equal(401))
			})
		})
	})

	// badssl.com is not ment for automated testing, therefore we will only run the tests when the label is provided
	// These tests check if umh-core can be deployed in environment with insecure TLS (e.g corporate proxies with there own self-signed certificates)
	Context("TLS Security", Label("tls"), func() {
		Describe("Certificate Validation", func() {
			// https://badssl.com/ is a website from the chromium project that provides various TLS test sites
			// These test sites help verify different TLS security scenarios
			testCases := map[string]string{
				"expired":        "https://expired.badssl.com",
				"wrong-host":     "https://wrong.host.badssl.com",
				"self-signed":    "https://self-signed.badssl.com",
				"untrusted-root": "https://untrusted-root.badssl.com",
				// The revoked certificate test is disabled, as go's crypto/tls requires the user to implement certificate revocation checking
				// See also: https://www.cossacklabs.com/blog/tls-validation-implementing-ocsp-and-crl-in-go/
				// "revoked":        "https://revoked.badssl.com",
				// The pinning test is disabled, as go's crypto/tls ignores HPKP (HTTP Public Key Pinning)
				// See also: https://blog.ordina-jworks.io/security/2018/02/12/HPKP-deprecated-what-now.html
				// "pinning-test":   "https://pinning-test.badssl.com"
			}

			for name, url := range testCases {
				It("should fail with secure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, false, log)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("certificate"))
				})

				It("should succeed with insecure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, true, log)
					Expect(err).ToNot(HaveOccurred())
				})
			}
		})
		Describe("Key exchange", Label("tls"), func() {
			// These will always fail, as the go crypto/tls does not allow them anymore (For good reasons)
			testCases := map[string]string{
				"dh480":             "https://dh480.badssl.com",
				"dh512":             "https://dh512.badssl.com",
				"dh1024":            "https://dh1024.badssl.com",
				"dh2048":            "https://dh2048.badssl.com",
				"dh-small-subgroup": "https://dh-small-subgroup.badssl.com",
				"dh-composite":      "https://dh-composite.badssl.com",
			}
			for name, url := range testCases {
				It("should fail with secure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, false, log)
					Expect(err).To(HaveOccurred())
				})

				It("should fail with insecure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, true, log)
					Expect(err).To(HaveOccurred())
				})
			}
		})

		Describe("Protocol", Label("tls"), func() {
			// By default go only allows TLS 1.2 and above
			testCases := map[string]string{
				"tls-v1-0": "https://tls-v1-0.badssl.com",
				"tls-v1-1": "https://tls-v1-1.badssl.com",
			}
			for name, url := range testCases {
				It("should fail with secure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, false, log)
					Expect(err).To(HaveOccurred())
				})

				It("should succeed with insecure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, true, log)
					Expect(err).ToNot(HaveOccurred())
				})
			}
		})

		Describe("Cipher Suites", Label("tls"), func() {
			// These cipher suites where removed from go, using them requires custom args when building the executable
			testCases := map[string]string{
				"rc4-md5": "https://rc4-md5.badssl.com",
				"rc4":     "https://rc4.badssl.com",
				"3des":    "https://3des.badssl.com",
				"null":    "https://null.badssl.com",
			}
			for name, url := range testCases {
				It("should fail with secure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, false, log)
					Expect(err).To(HaveOccurred())
				})

				It("should fail with insecure TLS for "+name, func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_, err := doHTTPRequestWithRetry(ctx, url, nil, nil, true, log)
					Expect(err).To(HaveOccurred())
				})
			}
		})
	})
})
