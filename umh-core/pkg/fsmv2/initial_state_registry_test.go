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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("InitialStateRegistry", func() {
	BeforeEach(func() {
		fsmv2.ResetInitialStateRegistry()
	})

	AfterEach(func() {
		fsmv2.ResetInitialStateRegistry()
	})

	It("registers and looks up a state successfully", func() {
		state := &testStoppedState{}
		fsmv2.RegisterInitialState("test-registry", state)

		found := fsmv2.LookupInitialState("test-registry")
		Expect(found).To(Equal(state))
	})

	It("panics on duplicate registration", func() {
		fsmv2.RegisterInitialState("test-dup", &testStoppedState{})
		Expect(func() {
			fsmv2.RegisterInitialState("test-dup", &testStoppedState{})
		}).To(PanicWith(ContainSubstring("duplicate registration")))
	})

	It("returns nil for unregistered worker type", func() {
		found := fsmv2.LookupInitialState("nonexistent")
		Expect(found).To(BeNil())
	})

	It("clears all registrations on reset", func() {
		fsmv2.RegisterInitialState("test-reset", &testStoppedState{})
		Expect(fsmv2.LookupInitialState("test-reset")).NotTo(BeNil())

		fsmv2.ResetInitialStateRegistry()
		Expect(fsmv2.LookupInitialState("test-reset")).To(BeNil())
	})

	It("panics on empty workerType", func() {
		Expect(func() {
			fsmv2.RegisterInitialState("", &testStoppedState{})
		}).To(PanicWith(ContainSubstring("must not be empty")))
	})

	It("panics on nil state", func() {
		Expect(func() {
			fsmv2.RegisterInitialState("test-nil", nil)
		}).To(PanicWith(ContainSubstring("must not be nil")))
	})
})

// testStoppedState is a minimal State implementation for registry tests.
type testStoppedState struct{}

func (s *testStoppedState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Stopped", nil)
}

func (s *testStoppedState) String() string {
	return "Stopped"
}

func (s *testStoppedState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopped
}
