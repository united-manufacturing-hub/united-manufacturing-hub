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

package hook

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func TestHook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hook Suite")
}

// testDeps builds a Deps with fresh channels, so a test can tell which run a
// function saw.
func testDeps() Deps {
	return Deps{
		Inbound:  make(chan handoff.Message, 1),
		Outbound: make(chan handoff.Message, 1),
		Replies:  make(chan *models.UMHMessage, 1),
		Config:   Config{RelayURL: "https://example.invalid"},
		Logger:   zap.NewNop().Sugar(),
	}
}

var _ = Describe("Registration", func() {
	var calls []string
	var seen []Deps

	BeforeEach(func() {
		registered = nil
		calls = nil
		seen = nil
	})

	It("checks that Run calls registered functions in registration order and hands each the same deps", func() {
		d := testDeps()
		Register(func(got Deps) error {
			calls = append(calls, "f1")
			seen = append(seen, got)
			return nil
		})
		Register(func(got Deps) error {
			calls = append(calls, "f2")
			seen = append(seen, got)
			return nil
		})

		Expect(Run(d)).To(Succeed())
		Expect(calls).To(Equal([]string{"f1", "f2"}))
		Expect(seen).To(HaveLen(2))
		Expect(seen[0].Inbound).To(Equal(seen[1].Inbound))
	})

	It("checks that Run stops at the first failing function and returns its error", func() {
		errX := errors.New("x")
		f2Called := false

		Register(func(Deps) error { return errX })
		Register(func(Deps) error {
			f2Called = true
			return nil
		})

		err := Run(testDeps())
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, errX)).To(BeTrue())
		Expect(f2Called).To(BeFalse())
	})

	It("checks that Run returns nil when nothing is registered", func() {
		Expect(Run(testDeps())).To(Succeed())
	})
})
