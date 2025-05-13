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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

var _ = Describe("Requester", func() {

	var log *zap.SugaredLogger

	// Set API_URL to the production API URL in order for the gock library to correctly
	// intercept the requests to the production API done by the code under test
	const apiUrl = "https://management.umh.app/api"

	BeforeEach(func() {
		log = logger.For("RequesterTest")
	})

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
			It("should return error for non 200 response", func() {
				_, err, _ := http.GetRequest[any](context.Background(), http.LoginEndpoint, header, &cookies, false, apiUrl, log)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("PostRequest", func() {
			It("should return error for non 200 response", func() {
				_, err, _ := http.PostRequest[any](context.Background(), http.LoginEndpoint, &data, header, &cookies, false, apiUrl, log)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("TLS Security", func() {
		// https://badssl.com/ is a website from the chromium project that provides various TLS test sites
		// The one we choose here is the untrusted-root.badssl.com site, which is a site that has a certificate
		// that is not trusted by default.
		const untrustedURL = "https://untrusted-root.badssl.com"

		It("should fail with secure TLS", func() {
			_, _, err := http.GetRequest[any](context.Background(), "/", nil, nil, false, untrustedURL, log)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("certificate"))
		})

		It("should succeed with insecure TLS", func() {
			_, _, err := http.GetRequest[any](context.Background(), "/", nil, nil, true, untrustedURL, log)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
