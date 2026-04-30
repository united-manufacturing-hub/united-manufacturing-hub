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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

type testDeps struct {
	Greeting string
}

type typedTestAction struct {
	called   bool
	received string
}

func (a *typedTestAction) Execute(ctx context.Context, deps *testDeps) error {
	a.called = true
	a.received = deps.Greeting

	return nil
}

func (a *typedTestAction) Name() string {
	return "typed_test_action"
}

type failingTestAction struct{}

func (a *failingTestAction) Execute(_ context.Context, _ *testDeps) error {
	return errors.New("action failed")
}

func (a *failingTestAction) Name() string {
	return "failing_action"
}

var _ = Describe("WrapAction", func() {
	It("delegates Execute to the typed action with correct deps", func() {
		typed := &typedTestAction{}
		wrapped := fsmv2.WrapAction[*testDeps](typed)

		deps := &testDeps{Greeting: "hello"}
		err := wrapped.Execute(context.Background(), deps)

		Expect(err).NotTo(HaveOccurred())
		Expect(typed.called).To(BeTrue())
		Expect(typed.received).To(Equal("hello"))
	})

	It("delegates Name to the typed action", func() {
		typed := &typedTestAction{}
		wrapped := fsmv2.WrapAction[*testDeps](typed)

		Expect(wrapped.Name()).To(Equal("typed_test_action"))
	})

	It("propagates errors from the typed action", func() {
		typed := &failingTestAction{}
		wrapped := fsmv2.WrapAction[*testDeps](typed)

		deps := &testDeps{}
		err := wrapped.Execute(context.Background(), deps)

		Expect(err).To(MatchError("action failed"))
	})

	It("panics with descriptive message on deps type mismatch", func() {
		typed := &typedTestAction{}
		wrapped := fsmv2.WrapAction[*testDeps](typed)

		Expect(func() {
			_ = wrapped.Execute(context.Background(), "wrong-type")
		}).To(PanicWith(ContainSubstring("deps type assertion failed")))
	})
})
