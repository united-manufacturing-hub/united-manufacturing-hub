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

package backoff_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

func TestBackoff(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backoff Suite")
}

var _ = Describe("CalculateDelay", func() {
	It("returns zero delay when no errors", func() {
		delay := backoff.CalculateDelay(0)
		Expect(delay).To(Equal(time.Duration(0)))
	})

	It("returns 2 seconds for 1 error (2^1)", func() {
		delay := backoff.CalculateDelay(1)
		Expect(delay).To(Equal(2 * time.Second))
	})

	It("returns 4 seconds for 2 errors (2^2)", func() {
		delay := backoff.CalculateDelay(2)
		Expect(delay).To(Equal(4 * time.Second))
	})

	It("returns 8 seconds for 3 errors (2^3)", func() {
		delay := backoff.CalculateDelay(3)
		Expect(delay).To(Equal(8 * time.Second))
	})

	It("returns 32 seconds for 5 errors (2^5)", func() {
		delay := backoff.CalculateDelay(5)
		Expect(delay).To(Equal(32 * time.Second))
	})

	It("caps at 60 seconds for 6+ errors", func() {
		delay := backoff.CalculateDelay(6)
		Expect(delay).To(Equal(60 * time.Second))
	})

	It("caps at 60 seconds for many errors", func() {
		delay := backoff.CalculateDelay(100)
		Expect(delay).To(Equal(60 * time.Second))
	})

	It("handles negative errors gracefully (returns 0)", func() {
		delay := backoff.CalculateDelay(-1)
		Expect(delay).To(Equal(time.Duration(0)))
	})
})

var _ = Describe("CalculateDelayForErrorType", func() {
	Context("when error type is ErrorTypeInstanceDeleted", func() {
		It("should return 5 minute delay regardless of consecutive errors", func() {
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeInstanceDeleted,
				1,
				0,
			)
			Expect(delay).To(Equal(5 * time.Minute))

			delay = backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeInstanceDeleted,
				10,
				0,
			)
			Expect(delay).To(Equal(5 * time.Minute))
		})
	})

	Context("when error type is ErrorTypeCloudflareChallenge", func() {
		It("should return 60 second delay", func() {
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeCloudflareChallenge,
				1,
				0,
			)
			Expect(delay).To(Equal(60 * time.Second))
		})
	})

	Context("when error type is ErrorTypeProxyBlock", func() {
		It("should return 60 second delay", func() {
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeProxyBlock,
				1,
				0,
			)
			Expect(delay).To(Equal(60 * time.Second))
		})
	})

	Context("when error type is ErrorTypeInvalidToken", func() {
		It("should return 60 second delay", func() {
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeInvalidToken,
				1,
				0,
			)
			Expect(delay).To(Equal(60 * time.Second))
		})
	})

	Context("when error type is ErrorTypeBackendRateLimit", func() {
		It("should escalate delays: 30s, 60s, 120s, 300s", func() {
			delays := []time.Duration{
				30 * time.Second,
				60 * time.Second,
				120 * time.Second,
				300 * time.Second,
			}
			for i, expected := range delays {
				delay := backoff.CalculateDelayForErrorType(
					httpTransport.ErrorTypeBackendRateLimit,
					i+1,
					0,
				)
				Expect(delay).To(Equal(expected), "at consecutive error %d", i+1)
			}
		})

		It("should cap at 300 seconds for many errors", func() {
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeBackendRateLimit,
				100,
				0,
			)
			Expect(delay).To(Equal(300 * time.Second))
		})
	})

	Context("when error type is ErrorTypeServerError", func() {
		It("should use exponential backoff", func() {
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeServerError,
				1,
				0,
			)
			Expect(delay).To(Equal(2 * time.Second))

			delay = backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeServerError,
				3,
				0,
			)
			Expect(delay).To(Equal(8 * time.Second))
		})
	})

	Context("when error type is ErrorTypeNetwork", func() {
		It("should use exponential backoff", func() {
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeNetwork,
				2,
				0,
			)
			Expect(delay).To(Equal(4 * time.Second))
		})
	})

	Context("when Retry-After header is provided", func() {
		It("should override default delay", func() {
			retryAfter := 120 * time.Second
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeBackendRateLimit,
				1,
				retryAfter,
			)
			Expect(delay).To(Equal(retryAfter))
		})

		It("should override for any error type", func() {
			retryAfter := 90 * time.Second
			delay := backoff.CalculateDelayForErrorType(
				httpTransport.ErrorTypeServerError,
				1,
				retryAfter,
			)
			Expect(delay).To(Equal(retryAfter))
		})
	})
})

var _ = Describe("ShouldStopRetrying", func() {
	It("should always return false", func() {
		errorTypes := []httpTransport.ErrorType{
			httpTransport.ErrorTypeNetwork,
			httpTransport.ErrorTypeServerError,
			httpTransport.ErrorTypeCloudflareChallenge,
			httpTransport.ErrorTypeProxyBlock,
			httpTransport.ErrorTypeInvalidToken,
			httpTransport.ErrorTypeBackendRateLimit,
			httpTransport.ErrorTypeInstanceDeleted,
		}
		for _, errType := range errorTypes {
			Expect(backoff.ShouldStopRetrying(errType, 1)).To(BeFalse())
			Expect(backoff.ShouldStopRetrying(errType, 100)).To(BeFalse())
		}
	})
})

var _ = Describe("ShouldResetTransport", func() {
	It("should return false when consecutive errors is 0", func() {
		Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeNetwork, 0)).To(BeFalse())
	})

	Context("for ErrorTypeNetwork", func() {
		It("should reset every 5 consecutive errors", func() {
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeNetwork, 4)).To(BeFalse())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeNetwork, 5)).To(BeTrue())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeNetwork, 6)).To(BeFalse())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeNetwork, 10)).To(BeTrue())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeNetwork, 15)).To(BeTrue())
		})
	})

	Context("for ErrorTypeServerError", func() {
		It("should reset every 10 consecutive errors", func() {
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeServerError, 5)).To(BeFalse())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeServerError, 10)).To(BeTrue())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeServerError, 15)).To(BeFalse())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeServerError, 20)).To(BeTrue())
		})
	})

	Context("for ErrorTypeCloudflareChallenge", func() {
		It("should reset every 10 consecutive errors", func() {
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeCloudflareChallenge, 10)).To(BeTrue())
			Expect(backoff.ShouldResetTransport(httpTransport.ErrorTypeCloudflareChallenge, 20)).To(BeTrue())
		})
	})

	Context("for other error types", func() {
		It("should not trigger transport reset", func() {
			otherTypes := []httpTransport.ErrorType{
				httpTransport.ErrorTypeInvalidToken,
				httpTransport.ErrorTypeBackendRateLimit,
				httpTransport.ErrorTypeProxyBlock,
				httpTransport.ErrorTypeInstanceDeleted,
			}
			for _, errType := range otherTypes {
				Expect(backoff.ShouldResetTransport(errType, 5)).To(BeFalse())
				Expect(backoff.ShouldResetTransport(errType, 10)).To(BeFalse())
				Expect(backoff.ShouldResetTransport(errType, 100)).To(BeFalse())
			}
		})
	})
})
