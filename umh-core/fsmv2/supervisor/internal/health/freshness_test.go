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

package health_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor/internal/health"
)

var _ = Describe("FreshnessChecker", func() {
	Context("when observation data is fresh", func() {
		It("should return true", func() {
			checker := health.NewFreshnessChecker(10*time.Second, 20*time.Second)

			observedAt := time.Now()

			Expect(checker.Check(observedAt)).To(BeTrue())
		})
	})

	Context("when observation data is stale", func() {
		It("should return false", func() {
			checker := health.NewFreshnessChecker(10*time.Second, 20*time.Second)

			observedAt := time.Now().Add(-15 * time.Second)

			Expect(checker.Check(observedAt)).To(BeFalse())
		})
	})

	Context("when observation data has timed out", func() {
		It("should detect timeout", func() {
			checker := health.NewFreshnessChecker(10*time.Second, 20*time.Second)

			observedAt := time.Now().Add(-25 * time.Second)

			Expect(checker.IsTimeout(observedAt)).To(BeTrue())
		})
	})
})
