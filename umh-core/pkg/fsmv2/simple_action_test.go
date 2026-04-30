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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

type simpleTestDeps struct {
	Value string
}

var _ = Describe("SimpleAction", func() {
	It("executes with correct typed deps", func() {
		var received string
		action := fsmv2.SimpleAction[*simpleTestDeps]("test-action", func(_ context.Context, deps *simpleTestDeps) error {
			received = deps.Value
			return nil
		})

		err := action.Execute(context.Background(), &simpleTestDeps{Value: "hello"})
		Expect(err).NotTo(HaveOccurred())
		Expect(received).To(Equal("hello"))
	})

	It("returns ctx.Err when context is already cancelled", func() {
		action := fsmv2.SimpleAction[*simpleTestDeps]("cancel-action", func(_ context.Context, _ *simpleTestDeps) error {
			Fail("fn should not be called when ctx is cancelled")
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := action.Execute(ctx, &simpleTestDeps{})
		Expect(err).To(MatchError(context.Canceled))
	})

	It("returns error on wrong deps type", func() {
		action := fsmv2.SimpleAction[*simpleTestDeps]("typed-action", func(_ context.Context, _ *simpleTestDeps) error {
			return nil
		})

		err := action.Execute(context.Background(), "wrong-type")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`SimpleAction "typed-action"`))
		Expect(err.Error()).To(ContainSubstring("expected deps type"))
	})

	It("returns Name() and String() as the registered name", func() {
		action := fsmv2.SimpleAction[*simpleTestDeps]("my-action", func(_ context.Context, _ *simpleTestDeps) error {
			return nil
		})

		Expect(action.Name()).To(Equal("my-action"))
		Expect(action.(fmt.Stringer).String()).To(Equal("my-action"))
	})

	It("propagates error from fn", func() {
		expectedErr := fmt.Errorf("something broke")
		action := fsmv2.SimpleAction[*simpleTestDeps]("err-action", func(_ context.Context, _ *simpleTestDeps) error {
			return expectedErr
		})

		err := action.Execute(context.Background(), &simpleTestDeps{})
		Expect(err).To(MatchError("something broke"))
	})
})
