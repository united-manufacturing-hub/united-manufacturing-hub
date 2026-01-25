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
	It("should return 5 minute delay for ErrorTypeInstanceDeleted", func() {
		delay := backoff.CalculateDelayForErrorType(
			httpTransport.ErrorTypeInstanceDeleted,
			1,
			0,
		)
		Expect(delay).To(Equal(5 * time.Minute))
	})
})
