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

package snapshot_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
)

func TestSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Snapshot Suite")
}

var _ = Describe("CommunicatorObservedState", func() {
	var observed *snapshot.CommunicatorObservedState

	BeforeEach(func() {
		observed = &snapshot.CommunicatorObservedState{}
	})

	Describe("IsTokenExpired", func() {
		Context("when token expires in 1 hour", func() {
			BeforeEach(func() {
				observed.JWTExpiry = time.Now().Add(1 * time.Hour)
			})

			It("should return false (not expired)", func() {
				expired := observed.IsTokenExpired()
				Expect(expired).To(BeFalse(), "Fresh token should not be expired")
			})
		})

		Context("when token expired 1 hour ago", func() {
			BeforeEach(func() {
				observed.JWTExpiry = time.Now().Add(-1 * time.Hour)
			})

			It("should return true (expired)", func() {
				expired := observed.IsTokenExpired()
				Expect(expired).To(BeTrue(), "Past expiration should be expired")
			})
		})

		Context("when token expires in 5 minutes (within 10-minute buffer)", func() {
			BeforeEach(func() {
				observed.JWTExpiry = time.Now().Add(5 * time.Minute)
			})

			It("should return true (considered expired for proactive refresh)", func() {
				expired := observed.IsTokenExpired()
				Expect(expired).To(BeTrue(), "Token expiring soon should be considered expired")
			})
		})

		Context("when token expires in 15 minutes (outside 10-minute buffer)", func() {
			BeforeEach(func() {
				observed.JWTExpiry = time.Now().Add(15 * time.Minute)
			})

			It("should return false (not yet in refresh window)", func() {
				expired := observed.IsTokenExpired()
				Expect(expired).To(BeFalse(), "Token with >10 minutes remaining should not be expired")
			})
		})

		Context("when no expiration is set", func() {
			It("should return false (zero time means no expiration tracking)", func() {
				expired := observed.IsTokenExpired()
				Expect(expired).To(BeFalse(), "Zero expiration time should not be considered expired")
			})
		})
	})

	Describe("GetConsecutiveErrors", func() {
		Context("when no errors have occurred", func() {
			It("should return 0", func() {
				Expect(observed.GetConsecutiveErrors()).To(Equal(0))
			})
		})

		Context("when ConsecutiveErrors is set to 1", func() {
			BeforeEach(func() {
				observed.ConsecutiveErrors = 1
			})

			It("should return 1", func() {
				Expect(observed.GetConsecutiveErrors()).To(Equal(1))
			})
		})

		Context("when multiple consecutive errors have occurred", func() {
			BeforeEach(func() {
				observed.ConsecutiveErrors = 5
			})

			It("should return the accumulated count", func() {
				Expect(observed.GetConsecutiveErrors()).To(Equal(5))
			})
		})
	})
})
