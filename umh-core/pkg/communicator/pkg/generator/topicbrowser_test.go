package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"go.uber.org/zap"
)

var _ = Describe("buildTopicBrowserAsDfc", func() {
	var (
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	DescribeTable("state to health category mapping",
		func(benthosFSMState string, expectedHealthCat models.HealthCategory, expectedDesiredState string) {
			observedState := &topicbrowserfsm.ObservedStateSnapshot{
				ServiceInfo: topicbrowsersvc.ServiceInfo{
					BenthosFSMState: benthosFSMState,
					StatusReason:    "test reason",
				},
			}

			instance := fsm.FSMInstanceSnapshot{
				ID:                "test-topicbrowser",
				CurrentState:      benthosFSMState,
				LastObservedState: observedState,
			}

			health, err := buildTopicBrowserAsDfc(instance, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).NotTo(BeNil())
			Expect(health.Category).To(Equal(expectedHealthCat))
			Expect(health.ObservedState).To(Equal(benthosFSMState))
			Expect(health.DesiredState).To(Equal(expectedDesiredState))
			Expect(health.Message).To(Equal("test reason"))
		},
		Entry("OperationalStateInitializing -> Neutral",
			"not_a_real_state",
			models.Neutral,
			"active",
		),
		Entry("OperationalStateActive -> Active",
			topicbrowserfsm.OperationalStateActive,
			models.Active,
			"active",
		),
		Entry("OperationalStateIdle -> Active",
			topicbrowserfsm.OperationalStateIdle,
			models.Active,
			"active",
		),
		Entry("OperationalStateDegradedBenthos -> Degraded",
			topicbrowserfsm.OperationalStateDegradedBenthos,
			models.Degraded,
			"active",
		),
		Entry("OperationalStateDegradedRedpanda -> Degraded",
			topicbrowserfsm.OperationalStateDegradedRedpanda,
			models.Degraded,
			"active",
		),
		Entry("OperationalStateStarting -> Neutral",
			topicbrowserfsm.OperationalStateStarting,
			models.Neutral,
			"active",
		),
		Entry("OperationalStateStopping -> Neutral",
			topicbrowserfsm.OperationalStateStopping,
			models.Neutral,
			"active",
		),
	)

	It("should return error when observed state is not topicbrowser snapshot", func() {
		// use wrong type for observed state
		invalidObservedState := &benthosfsm.BenthosObservedStateSnapshot{}

		instance := fsm.FSMInstanceSnapshot{
			ID:                "test-topicbrowser",
			CurrentState:      topicbrowserfsm.OperationalStateActive,
			LastObservedState: invalidObservedState,
		}

		health, err := buildTopicBrowserAsDfc(instance, logger)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("last observed state is not a topic browser observed state snapshot"))
		Expect(health).To(BeNil())
	})

	It("should preserve status reason in health message", func() {
		expectedMessage := "custom status reason for testing"
		observedState := &topicbrowserfsm.ObservedStateSnapshot{
			ServiceInfo: topicbrowsersvc.ServiceInfo{
				BenthosFSMState: topicbrowserfsm.OperationalStateActive,
				StatusReason:    expectedMessage,
			},
		}

		instance := fsm.FSMInstanceSnapshot{
			ID:                "test-topicbrowser",
			CurrentState:      topicbrowserfsm.OperationalStateActive,
			LastObservedState: observedState,
		}

		health, err := buildTopicBrowserAsDfc(instance, logger)

		Expect(err).NotTo(HaveOccurred())
		Expect(health.Message).To(Equal(expectedMessage))
	})
})
