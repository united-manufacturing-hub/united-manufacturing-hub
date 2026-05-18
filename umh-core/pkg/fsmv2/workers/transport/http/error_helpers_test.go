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
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

var _ = Describe("ExtractErrorType", func() {
	It("extracts type and RetryAfter from a TransportError", func() {
		inner := errors.New("rate limited")
		transportErr := httpTransport.NewTransportErrorForTest(
			types.ErrorTypeBackendRateLimit, 429, "rate limited", 30*time.Second, inner,
		)

		errType, retryAfter := types.ExtractErrorType(transportErr)
		Expect(errType).To(Equal(types.ErrorTypeBackendRateLimit))
		Expect(retryAfter).To(Equal(30 * time.Second))
	})

	It("extracts type from a wrapped TransportError", func() {
		inner := errors.New("server error")
		transportErr := httpTransport.NewTransportErrorForTest(
			types.ErrorTypeServerError, 500, "server error", 0, inner,
		)
		wrapped := fmt.Errorf("operation failed: %w", transportErr)

		errType, retryAfter := types.ExtractErrorType(wrapped)
		Expect(errType).To(Equal(types.ErrorTypeServerError))
		Expect(retryAfter).To(BeZero())
	})

	It("defaults to ErrorTypeUnknown for non-TransportError", func() {
		plainErr := errors.New("connection refused")

		errType, retryAfter := types.ExtractErrorType(plainErr)
		Expect(errType).To(Equal(types.ErrorTypeUnknown))
		Expect(retryAfter).To(BeZero())
	})
})

var _ = Describe("CounterForErrorType", func() {
	DescribeTable("maps each ErrorType to its Prometheus counter",
		func(errType types.ErrorType, expected depspkg.CounterName) {
			Expect(types.CounterForErrorType(errType)).To(Equal(expected))
		},
		Entry("CloudflareChallenge", types.ErrorTypeCloudflareChallenge, depspkg.CounterCloudflareErrorsTotal),
		Entry("BackendRateLimit", types.ErrorTypeBackendRateLimit, depspkg.CounterBackendRateLimitErrorsTotal),
		Entry("InvalidToken", types.ErrorTypeInvalidToken, depspkg.CounterAuthFailuresTotal),
		Entry("InstanceDeleted", types.ErrorTypeInstanceDeleted, depspkg.CounterInstanceDeletedTotal),
		Entry("ServerError", types.ErrorTypeServerError, depspkg.CounterServerErrorsTotal),
		Entry("ProxyBlock", types.ErrorTypeProxyBlock, depspkg.CounterProxyBlockErrorsTotal),
		Entry("Network", types.ErrorTypeNetwork, depspkg.CounterNetworkErrorsTotal),
		Entry("ChannelFull defaults to network", types.ErrorTypeChannelFull, depspkg.CounterNetworkErrorsTotal),
		Entry("Unknown defaults to network", types.ErrorTypeUnknown, depspkg.CounterNetworkErrorsTotal),
	)

	It("defaults to network counter for out-of-range ErrorType", func() {
		outOfRange := types.ErrorType(999)
		Expect(types.CounterForErrorType(outOfRange)).To(Equal(depspkg.CounterNetworkErrorsTotal))
	})
})
