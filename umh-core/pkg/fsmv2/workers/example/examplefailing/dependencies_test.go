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

package examplefailing_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap"

	fsmdeps "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
)

func TestDependencies(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Examplefailing Dependencies Suite")
}

var _ = Describe("FailingDependencies Observation-Based Recovery", func() {
	var failDeps *examplefailing.FailingDependencies

	BeforeEach(func() {
		zapLogger, _ := zap.NewDevelopment()
		logger := zapLogger.Sugar()
		pool := &examplefailing.DefaultConnectionPool{}
		identity := fsmdeps.Identity{ID: "test-worker", WorkerType: "examplefailing"}
		failDeps = examplefailing.NewFailingDependencies(pool, logger, nil, identity)
	})

	Describe("SetRecoveryDelayObservations and GetRecoveryDelayObservations", func() {
		It("should set and get recovery delay observations", func() {
			failDeps.SetRecoveryDelayObservations(5)
			Expect(failDeps.GetRecoveryDelayObservations()).To(Equal(5))
		})

		It("should default to 0", func() {
			Expect(failDeps.GetRecoveryDelayObservations()).To(Equal(0))
		})
	})

	Describe("IncrementObservationsSinceFailure", func() {
		It("should increment the counter and return new value", func() {
			result := failDeps.IncrementObservationsSinceFailure()
			Expect(result).To(Equal(1))

			result = failDeps.IncrementObservationsSinceFailure()
			Expect(result).To(Equal(2))
		})
	})

	Describe("GetObservationsSinceFailure", func() {
		It("should return current counter value", func() {
			Expect(failDeps.GetObservationsSinceFailure()).To(Equal(0))

			failDeps.IncrementObservationsSinceFailure()
			Expect(failDeps.GetObservationsSinceFailure()).To(Equal(1))
		})
	})

	Describe("ResetObservationsSinceFailure", func() {
		It("should reset counter to 0", func() {
			failDeps.IncrementObservationsSinceFailure()
			failDeps.IncrementObservationsSinceFailure()
			Expect(failDeps.GetObservationsSinceFailure()).To(Equal(2))

			failDeps.ResetObservationsSinceFailure()
			Expect(failDeps.GetObservationsSinceFailure()).To(Equal(0))
		})
	})

	Describe("ShouldDelayRecovery (observation-based)", func() {
		Context("when recoveryDelayObservations is 0", func() {
			It("should return false", func() {
				failDeps.SetRecoveryDelayObservations(0)
				Expect(failDeps.ShouldDelayRecovery()).To(BeFalse())
			})
		})

		Context("when observationsSinceFailure < recoveryDelayObservations", func() {
			It("should return true", func() {
				failDeps.SetRecoveryDelayObservations(3)
				failDeps.IncrementObservationsSinceFailure() // 1 observation
				Expect(failDeps.ShouldDelayRecovery()).To(BeTrue())
			})
		})

		Context("when observationsSinceFailure >= recoveryDelayObservations", func() {
			It("should return false when equal", func() {
				failDeps.SetRecoveryDelayObservations(2)
				failDeps.IncrementObservationsSinceFailure() // 1
				failDeps.IncrementObservationsSinceFailure() // 2
				Expect(failDeps.ShouldDelayRecovery()).To(BeFalse())
			})

			It("should return false when greater", func() {
				failDeps.SetRecoveryDelayObservations(2)
				failDeps.IncrementObservationsSinceFailure() // 1
				failDeps.IncrementObservationsSinceFailure() // 2
				failDeps.IncrementObservationsSinceFailure() // 3
				Expect(failDeps.ShouldDelayRecovery()).To(BeFalse())
			})
		})
	})
})
