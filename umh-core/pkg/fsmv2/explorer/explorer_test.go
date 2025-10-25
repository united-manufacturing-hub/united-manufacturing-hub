// Copyright 2025 UMH Systems GmbH
package explorer_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/explorer"
)

func TestExplorer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Explorer Suite")
}

var _ = Describe("Explorer", func() {
	Describe("basic functionality", func() {
		It("should create explorer with scenario", func() {
			scenario := &mockScenario{}
			exp := explorer.New(scenario)

			Expect(exp).ToNot(BeNil())
		})

		It("should execute manual tick", func() {
			scenario := &mockScenario{
				tickCount: 0,
			}
			exp := explorer.New(scenario)

			err := exp.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(scenario.tickCount).To(Equal(1))
		})

		It("should display current state", func() {
			scenario := &mockScenario{
				currentState: "TestState",
				stateReason:  "test reason",
			}
			exp := explorer.New(scenario)

			state, reason := exp.GetCurrentState()
			Expect(state).To(Equal("TestState"))
			Expect(reason).To(Equal("test reason"))
		})
	})

	Describe("command parsing", func() {
		It("should parse tick command", func() {
			cmd := explorer.ParseCommand("t")
			Expect(cmd.Type).To(Equal(explorer.CommandTick))
		})

		It("should parse state command", func() {
			cmd := explorer.ParseCommand("s")
			Expect(cmd.Type).To(Equal(explorer.CommandShowState))
		})

		It("should parse observed command", func() {
			cmd := explorer.ParseCommand("o")
			Expect(cmd.Type).To(Equal(explorer.CommandShowObserved))
		})

		It("should parse desired command", func() {
			cmd := explorer.ParseCommand("d")
			Expect(cmd.Type).To(Equal(explorer.CommandShowDesired))
		})

		It("should parse shutdown command", func() {
			cmd := explorer.ParseCommand("shutdown")
			Expect(cmd.Type).To(Equal(explorer.CommandShutdown))
		})

		It("should parse quit command", func() {
			cmd := explorer.ParseCommand("q")
			Expect(cmd.Type).To(Equal(explorer.CommandQuit))
		})

		It("should return unknown for invalid command", func() {
			cmd := explorer.ParseCommand("invalid")
			Expect(cmd.Type).To(Equal(explorer.CommandUnknown))
		})
	})

	Describe("command execution", func() {
		It("should execute tick command", func() {
			scenario := &mockScenario{tickCount: 0}
			exp := explorer.New(scenario)

			cmd := explorer.Command{Type: explorer.CommandTick}
			result := exp.ExecuteCommand(context.Background(), cmd)

			Expect(result.Success).To(BeTrue())
			Expect(scenario.tickCount).To(Equal(1))
		})

		It("should execute show state command", func() {
			scenario := &mockScenario{
				currentState: "Running",
				stateReason:  "healthy",
			}
			exp := explorer.New(scenario)

			cmd := explorer.Command{Type: explorer.CommandShowState}
			result := exp.ExecuteCommand(context.Background(), cmd)

			Expect(result.Success).To(BeTrue())
			Expect(result.Message).To(ContainSubstring("Running"))
			Expect(result.Message).To(ContainSubstring("healthy"))
		})

		It("should execute shutdown command", func() {
			scenario := &mockScenario{}
			exp := explorer.New(scenario)

			cmd := explorer.Command{Type: explorer.CommandShutdown}
			result := exp.ExecuteCommand(context.Background(), cmd)

			Expect(result.Success).To(BeTrue())
			Expect(scenario.shutdownInjected).To(BeTrue())
		})

		It("should return error for unknown command", func() {
			scenario := &mockScenario{}
			exp := explorer.New(scenario)

			cmd := explorer.Command{Type: explorer.CommandUnknown}
			result := exp.ExecuteCommand(context.Background(), cmd)

			Expect(result.Success).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Unknown"))
		})
	})
})

type mockScenario struct {
	tickCount        int
	currentState     string
	stateReason      string
	shutdownInjected bool
}

func (m *mockScenario) Setup(ctx context.Context) error {
	return nil
}

func (m *mockScenario) Tick(ctx context.Context) error {
	m.tickCount++
	return nil
}

func (m *mockScenario) GetCurrentState() (string, string) {
	return m.currentState, m.stateReason
}

func (m *mockScenario) GetObservedState() interface{} {
	return nil
}

func (m *mockScenario) GetDesiredState() interface{} {
	return nil
}

func (m *mockScenario) InjectShutdown() error {
	m.shutdownInjected = true
	return nil
}

func (m *mockScenario) Cleanup(ctx context.Context) error {
	return nil
}
