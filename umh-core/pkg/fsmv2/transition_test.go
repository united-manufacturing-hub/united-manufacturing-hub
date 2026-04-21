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
	"errors"
	"fmt"

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

type fakeTypedDeps struct {
	marker string
}

type fakeTypedAction struct {
	returnError error
	called      *bool
	name        string
	wantMarker  string
}

func (a *fakeTypedAction) Execute(_ context.Context, deps fakeTypedDeps) error {
	if a.called != nil {
		*a.called = true
	}
	if a.wantMarker != "" && deps.marker != a.wantMarker {
		return fmt.Errorf("unexpected marker: got %q, want %q", deps.marker, a.wantMarker)
	}
	return a.returnError
}

func (a *fakeTypedAction) Name() string { return a.name }

type fakeNonAction struct{}

type fakePtrDepsAction struct {
	sawNil *bool
	name   string
}

func (a *fakePtrDepsAction) Execute(_ context.Context, deps *fakeTypedDeps) error {
	if a.sawNil != nil {
		*a.sawNil = (deps == nil)
	}
	return nil
}

func (a *fakePtrDepsAction) Name() string { return a.name }

type fakeIntDepsAction struct {
	name string
}

func (a *fakeIntDepsAction) Execute(_ context.Context, _ int) error { return nil }
func (a *fakeIntDepsAction) Name() string                           { return a.name }

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

	It("panics with an auto-wrap diagnostic for non-Action values", func() {
		state := &fakeTransitionState{}

		Expect(func() {
			fsmv2.Transition(state, fsmv2.SignalNone, 42, "wrong-type")
		}).To(PanicWith(And(
			ContainSubstring("fsmv2.Transition auto-wrap"),
			ContainSubstring("int"),
		)))
	})

	It("panics when a struct lacks Execute/Name methods", func() {
		state := &fakeTransitionState{}

		Expect(func() {
			fsmv2.Transition(state, fsmv2.SignalNone, &fakeNonAction{}, "no-methods")
		}).To(PanicWith(And(
			ContainSubstring("fsmv2.Transition auto-wrap"),
			ContainSubstring("fakeNonAction"),
		)))
	})

	It("auto-wraps a typed Action[TDeps] and delegates Execute", func() {
		state := &fakeTransitionState{}
		called := false
		action := &fakeTypedAction{
			name:       "typed",
			wantMarker: "hello",
			called:     &called,
		}

		result := fsmv2.Transition(state, fsmv2.SignalNone, action, "typed-ok")

		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("typed"))

		err := result.Action.Execute(context.Background(), fakeTypedDeps{marker: "hello"})
		Expect(err).NotTo(HaveOccurred())
		Expect(called).To(BeTrue())
	})

	It("propagates errors from the inner typed Action", func() {
		state := &fakeTransitionState{}
		sentinel := errors.New("inner boom")
		action := &fakeTypedAction{
			name:        "typed-err",
			returnError: sentinel,
		}

		result := fsmv2.Transition(state, fsmv2.SignalNone, action, "typed-err")

		err := result.Action.Execute(context.Background(), fakeTypedDeps{})
		Expect(err).To(MatchError(sentinel))
	})

	It("returns a descriptive error when deps type is not assignable to TDeps", func() {
		state := &fakeTransitionState{}
		called := false
		action := &fakeTypedAction{name: "typed-mismatch", called: &called}

		result := fsmv2.Transition(state, fsmv2.SignalNone, action, "deps-mismatch")

		err := result.Action.Execute(context.Background(), 42)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not assignable"))
		Expect(err.Error()).To(ContainSubstring("typed-mismatch"))
		Expect(called).To(BeFalse())
	})

	It("panics with a typed-nil diagnostic when a typed nil action is passed", func() {
		state := &fakeTransitionState{}
		var a *fakeTypedAction = nil

		Expect(func() {
			fsmv2.Transition(state, fsmv2.SignalNone, a, "typed-nil")
		}).To(PanicWith(And(
			ContainSubstring("typed nil action"),
			ContainSubstring("fakeTypedAction"),
		)))
	})

	It("accepts a nil deps value when TDeps is a nilable pointer kind", func() {
		state := &fakeTransitionState{}
		sawNil := false
		action := &fakePtrDepsAction{name: "ptr-deps", sawNil: &sawNil}

		result := fsmv2.Transition(state, fsmv2.SignalNone, action, "ptr-nil-ok")

		err := result.Action.Execute(context.Background(), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(sawNil).To(BeTrue())
	})

	It("returns an error when nil deps is passed to a non-nilable TDeps", func() {
		state := &fakeTransitionState{}
		action := &fakeIntDepsAction{name: "int-deps"}

		result := fsmv2.Transition(state, fsmv2.SignalNone, action, "int-nil-fail")

		err := result.Action.Execute(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nil deps not assignable"))
	})
})
