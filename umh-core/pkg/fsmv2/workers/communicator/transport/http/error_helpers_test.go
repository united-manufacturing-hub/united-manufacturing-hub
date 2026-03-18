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

package transport_test

import (
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

var _ = Describe("ExtractErrorType", func() {
	It("extracts type and RetryAfter from a TransportError", func() {
		inner := errors.New("rate limited")
		transportErr := httpTransport.NewTransportErrorForTest(
			httpTransport.ErrorTypeBackendRateLimit, 429, "rate limited", 30*time.Second, inner,
		)

		errType, retryAfter := httpTransport.ExtractErrorType(transportErr)
		Expect(errType).To(Equal(httpTransport.ErrorTypeBackendRateLimit))
		Expect(retryAfter).To(Equal(30 * time.Second))
	})

	It("extracts type from a wrapped TransportError", func() {
		inner := errors.New("server error")
		transportErr := httpTransport.NewTransportErrorForTest(
			httpTransport.ErrorTypeServerError, 500, "server error", 0, inner,
		)
		wrapped := fmt.Errorf("operation failed: %w", transportErr)

		errType, retryAfter := httpTransport.ExtractErrorType(wrapped)
		Expect(errType).To(Equal(httpTransport.ErrorTypeServerError))
		Expect(retryAfter).To(BeZero())
	})

	It("defaults to ErrorTypeUnknown for non-TransportError", func() {
		plainErr := errors.New("connection refused")

		errType, retryAfter := httpTransport.ExtractErrorType(plainErr)
		Expect(errType).To(Equal(httpTransport.ErrorTypeUnknown))
		Expect(retryAfter).To(BeZero())
	})
})

var _ = Describe("CounterForErrorType", func() {
	DescribeTable("maps each ErrorType to its Prometheus counter",
		func(errType httpTransport.ErrorType, expected depspkg.CounterName) {
			Expect(httpTransport.CounterForErrorType(errType)).To(Equal(expected))
		},
		Entry("CloudflareChallenge", httpTransport.ErrorTypeCloudflareChallenge, depspkg.CounterCloudflareErrorsTotal),
		Entry("BackendRateLimit", httpTransport.ErrorTypeBackendRateLimit, depspkg.CounterBackendRateLimitErrorsTotal),
		Entry("InvalidToken", httpTransport.ErrorTypeInvalidToken, depspkg.CounterAuthFailuresTotal),
		Entry("InstanceDeleted", httpTransport.ErrorTypeInstanceDeleted, depspkg.CounterInstanceDeletedTotal),
		Entry("ServerError", httpTransport.ErrorTypeServerError, depspkg.CounterServerErrorsTotal),
		Entry("ProxyBlock", httpTransport.ErrorTypeProxyBlock, depspkg.CounterProxyBlockErrorsTotal),
		Entry("Network", httpTransport.ErrorTypeNetwork, depspkg.CounterNetworkErrorsTotal),
		Entry("Unknown defaults to network", httpTransport.ErrorTypeUnknown, depspkg.CounterNetworkErrorsTotal),
	)

	It("defaults to network counter for out-of-range ErrorType", func() {
		outOfRange := httpTransport.ErrorType(999)
		Expect(httpTransport.CounterForErrorType(outOfRange)).To(Equal(depspkg.CounterNetworkErrorsTotal))
	})
})
