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

package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	redpandasvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"go.uber.org/zap"
)

var _ = Describe("buildRedpanda", func() {
	var (
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	DescribeTable("state to health category mapping",
		func(currentState string, expectedHealthCat models.HealthCategory, expectedDesiredState string) {
			observedState := &redpandafsm.RedpandaObservedStateSnapshot{
				ServiceInfoSnapshot: redpandasvc.ServiceInfo{
					StatusReason: "test reason",
				},
			}

			instance := fsm.FSMInstanceSnapshot{
				ID:                "test-redpanda",
				CurrentState:      currentState,
				DesiredState:      expectedDesiredState,
				LastObservedState: observedState,
			}

			redpanda, err := buildRedpanda(instance, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(redpanda.Health).NotTo(BeNil())
			Expect(redpanda.Health.Category).To(Equal(expectedHealthCat))
			Expect(redpanda.Health.ObservedState).To(Equal(currentState))
			Expect(redpanda.Health.DesiredState).To(Equal(expectedDesiredState))
			Expect(redpanda.Health.Message).To(Equal("test reason"))
		},
		Entry("OperationalStateActive -> Active",
			redpandafsm.OperationalStateActive,
			models.Active,
			"active",
		),
		Entry("OperationalStateIdle -> Active",
			redpandafsm.OperationalStateIdle,
			models.Active,
			"active",
		),
		Entry("OperationalStateDegraded -> Degraded",
			redpandafsm.OperationalStateDegraded,
			models.Degraded,
			"active",
		),
		Entry("OperationalStateStarting -> Neutral",
			redpandafsm.OperationalStateStarting,
			models.Neutral,
			"active",
		),
		Entry("OperationalStateStopping -> Neutral",
			redpandafsm.OperationalStateStopping,
			models.Neutral,
			"active",
		),
		Entry("OperationalStateStopped -> Neutral",
			redpandafsm.OperationalStateStopped,
			models.Neutral,
			"stopped",
		),
		Entry("Unknown state -> Neutral",
			"not_a_real_state",
			models.Neutral,
			"active",
		),
	)

	It("should return error when observed state is not redpanda snapshot", func() {
		invalidObservedState := &benthosfsm.BenthosObservedStateSnapshot{}

		instance := fsm.FSMInstanceSnapshot{
			ID:                "test-redpanda",
			CurrentState:      redpandafsm.OperationalStateActive,
			DesiredState:      "active",
			LastObservedState: invalidObservedState,
		}

		redpanda, err := buildRedpanda(instance, logger)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid observed-state"))
		Expect(redpanda).To(Equal(models.Redpanda{}))
	})

	It("should return error when observed state is nil", func() {
		instance := fsm.FSMInstanceSnapshot{
			ID:                "test-redpanda",
			CurrentState:      redpandafsm.OperationalStateActive,
			DesiredState:      "active",
			LastObservedState: nil,
		}

		redpanda, err := buildRedpanda(instance, logger)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid observed-state"))
		Expect(redpanda).To(Equal(models.Redpanda{}))
	})

	It("should preserve status reason in health message", func() {
		expectedMessage := "custom status reason for testing"
		observedState := &redpandafsm.RedpandaObservedStateSnapshot{
			ServiceInfoSnapshot: redpandasvc.ServiceInfo{
				StatusReason: expectedMessage,
				// RedpandaStatus is empty, so MetricsState will be nil
			},
		}

		instance := fsm.FSMInstanceSnapshot{
			ID:                "test-redpanda",
			CurrentState:      redpandafsm.OperationalStateActive,
			DesiredState:      "active",
			LastObservedState: observedState,
		}

		redpanda, err := buildRedpanda(instance, logger)

		Expect(err).NotTo(HaveOccurred())
		Expect(redpanda.Health.Message).To(Equal(expectedMessage))
	})
})
