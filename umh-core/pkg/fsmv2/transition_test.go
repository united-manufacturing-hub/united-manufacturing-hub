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

package fsmv2_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

type fakeTransitionState struct{}

func (s *fakeTransitionState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.NextResult[any, any]{State: s}
}

func (s *fakeTransitionState) String() string { return "fake" }

func (s *fakeTransitionState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopped
}

type fakeTransitionAction struct {
	name string
}

func (a *fakeTransitionAction) Execute(_ context.Context, _ any) error { return nil }
func (a *fakeTransitionAction) Name() string                           { return a.name }

var _ = Describe("Transition", func() {
	It("returns a NextResult with matching fields for a simple transition", func() {
		state := &fakeTransitionState{}

		result := fsmv2.Transition(state, fsmv2.SignalNone, nil, "reason")

		Expect(result.State).To(BeIdenticalTo(state))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).To(BeNil())
		Expect(result.Reason).To(Equal("reason"))
	})

	It("propagates the action instance through to NextResult", func() {
		state := &fakeTransitionState{}
		action := &fakeTransitionAction{name: "noop"}

		result := fsmv2.Transition(state, fsmv2.SignalNone, action, "with-action")

		Expect(result.Action).To(BeIdenticalTo(action))
	})

	It("propagates SignalNeedsRestart to NextResult", func() {
		state := &fakeTransitionState{}

		result := fsmv2.Transition(state, fsmv2.SignalNeedsRestart, nil, "restart")

		Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRestart))
	})

	It("panics with a C3-pointing message for unsupported action types", func() {
		state := &fakeTransitionState{}

		Expect(func() {
			fsmv2.Transition(state, fsmv2.SignalNone, 42, "wrong-type")
		}).To(PanicWith(ContainSubstring("PR1 C3 adds auto-wrap")))
	})
})
