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

package validator_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/validator"
)

const activeStateTemplate = `package state

type RunningState struct {
	helpers.RunningHealthyBase
}

func (s *RunningState) Next(snapAny any) any {
	snap := convert(snapAny)

	if %s {
		return stop()
	}

	return s
}
`

const activeStateWithoutConditional = `package state

type RunningState struct {
	helpers.RunningHealthyBase
}

func (s *RunningState) Next(snapAny any) any {
	_ = convert(snapAny)

	return s
}
`

const stoppingStateCheckingNothing = `package state

type TryingToStopState struct {
	helpers.StoppingBase
}

func (s *TryingToStopState) Next(snapAny any) any {
	snap := convert(snapAny)

	if snap.Status.Done {
		return Transition(&StoppedState{})
	}

	return Transition(s)
}
`

const stoppedStateTemplate = `package state

type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) any {
	snap := convert(snapAny)

%s
}
`

const stoppedStateHandlingDisable = `	if snap.IsShutdownRequested {
		return Transition(s, SignalNeedsRemoval)
	}

	if snap.IsDisabled {
		return Transition(s, SignalNone)
	}

	return Transition(&TryingToStartState{}, SignalNone)`

const stoppedStateCheckingOnlyShutdown = `	if snap.IsShutdownRequested {
		return Transition(s, SignalNeedsRemoval)
	}

	return Transition(&TryingToStartState{}, SignalNone)`

const stoppedStateRemovingOnShouldStop = `	if snap.ShouldStop() {
		return Transition(s, SignalNeedsRemoval)
	}

	return Transition(&TryingToStartState{}, SignalNone)`

const stoppedStateRemovingOnShouldStopBeforeDisable = `	if snap.ShouldStop() {
		return Transition(s, SignalNeedsRemoval)
	}

	if snap.IsDisabled {
		return Transition(s, SignalNone)
	}

	return Transition(&TryingToStartState{}, SignalNone)`

const stoppedStateReturningOnlySometimesWhileDisabled = `	if snap.IsShutdownRequested {
		return Transition(s, SignalNeedsRemoval)
	}

	if snap.IsDisabled {
		if snap.Ready {
			return Transition(s, SignalNone)
		}
	}

	return Transition(&TryingToStartState{}, SignalNone)`

const stoppedStateCheckingDisableAfterStart = `	if snap.IsShutdownRequested {
		return Transition(s, SignalNeedsRemoval)
	}

	if snap.Ready {
		return Transition(&TryingToStartState{}, SignalNone)
	}

	if snap.IsDisabled {
		return Transition(s, SignalNone)
	}

	return Transition(s, SignalNone)`

const stoppedStateStartingWhileDisabled = `	if snap.IsDisabled {
		return Transition(&TryingToStartState{}, SignalNone)
	}

	return Transition(s, SignalNone)`

const stoppedStateRemovingWhileDisabled = `	if snap.IsDisabled {
		return Transition(s, SignalNeedsRemoval)
	}

	return Transition(&TryingToStartState{}, SignalNone)`

// writeStateFile writes source as workers/demo/state/<fileName> under a new
// temporary base directory and returns that base directory.
func writeStateFile(fileName string, source string) string {
	baseDir := GinkgoT().TempDir()
	stateDir := filepath.Join(baseDir, "workers", "demo", "state")
	Expect(os.MkdirAll(stateDir, 0o755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(stateDir, fileName), []byte(source), 0o644)).To(Succeed())

	return baseDir
}

func activeState(condition string) string {
	return fmt.Sprintf(activeStateTemplate, condition)
}

func stoppedState(body string) string {
	return fmt.Sprintf(stoppedStateTemplate, body)
}

var _ = Describe("ValidateStopCheckFirst", func() {
	DescribeTable("rejects an active state whose first condition does not stop on both reasons",
		func(condition string) {
			baseDir := writeStateFile("state_running.go", activeState(condition))

			violations := validator.ValidateStopCheckFirst(baseDir)

			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal("STOP_CHECK_NOT_FIRST"))
		},
		Entry("shutdown only", "snap.IsShutdownRequested"),
		Entry("shutdown and disable joined with &&", "snap.IsShutdownRequested && snap.IsDisabled"),
		Entry("disable negated", "snap.IsShutdownRequested || !snap.IsDisabled"),
		Entry("ShouldStop negated", "!snap.ShouldStop()"),
	)

	DescribeTable("accepts an active state whose first condition stops on both reasons",
		func(condition string) {
			baseDir := writeStateFile("state_running.go", activeState(condition))

			Expect(validator.ValidateStopCheckFirst(baseDir)).To(BeEmpty())
		},
		Entry("ShouldStop()", "snap.ShouldStop()"),
		Entry("shutdown or disable", "snap.IsShutdownRequested || snap.IsDisabled"),
		Entry("disable or shutdown", "(snap.IsDisabled || snap.IsShutdownRequested)"),
	)

	It("rejects an active state whose Next() has no conditional", func() {
		baseDir := writeStateFile("state_running.go", activeStateWithoutConditional)

		violations := validator.ValidateStopCheckFirst(baseDir)

		Expect(violations).To(HaveLen(1))
		Expect(violations[0].Type).To(Equal("STOP_CHECK_NOT_FIRST"))
	})

	It("exempts a state that embeds StoppingBase", func() {
		baseDir := writeStateFile("state_trying_to_stop.go", stoppingStateCheckingNothing)

		Expect(validator.ValidateStopCheckFirst(baseDir)).To(BeEmpty())
	})

	It("checks an active state whose file name contains stopping", func() {
		baseDir := writeStateFile("state_stopping_children.go", activeState("snap.IsShutdownRequested"))

		Expect(validator.ValidateStopCheckFirst(baseDir)).To(HaveLen(1))
	})
})

var _ = Describe("ValidateStoppedStateHandlesDisable", func() {
	It("accepts a Stopped state that stays in Stopped while disabled", func() {
		baseDir := writeStateFile("state_stopped.go", stoppedState(stoppedStateHandlingDisable))

		Expect(validator.ValidateStoppedStateHandlesDisable(baseDir)).To(BeEmpty())
	})

	DescribeTable("rejects a Stopped state that does not keep a disabled worker in Stopped",
		func(body string) {
			baseDir := writeStateFile("state_stopped.go", stoppedState(body))

			violations := validator.ValidateStoppedStateHandlesDisable(baseDir)

			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal("STOPPED_IGNORES_DISABLE"))
		},
		Entry("no disable branch", stoppedStateCheckingOnlyShutdown),
		Entry("removal on ShouldStop()", stoppedStateRemovingOnShouldStop),
		Entry("removal on ShouldStop() before a disable branch", stoppedStateRemovingOnShouldStopBeforeDisable),
		Entry("disable branch that returns on only some paths", stoppedStateReturningOnlySometimesWhileDisabled),
		Entry("disable branch after a start transition", stoppedStateCheckingDisableAfterStart),
		Entry("disable branch that starts the worker", stoppedStateStartingWhileDisabled),
		Entry("disable branch that signals removal", stoppedStateRemovingWhileDisabled),
	)

	It("checks a Stopped state whose file name does not contain stopped", func() {
		baseDir := writeStateFile("state_idle.go", stoppedState(stoppedStateCheckingOnlyShutdown))

		Expect(validator.ValidateStoppedStateHandlesDisable(baseDir)).To(HaveLen(1))
	})

	It("ignores active states", func() {
		baseDir := writeStateFile("state_running.go", activeState("snap.IsShutdownRequested"))

		Expect(validator.ValidateStoppedStateHandlesDisable(baseDir)).To(BeEmpty())
	})
})
