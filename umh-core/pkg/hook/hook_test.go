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
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
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
		Register(func(got Deps) (Result, error) {
			calls = append(calls, "f1")
			seen = append(seen, got)
			return Result{}, nil
		})
		Register(func(got Deps) (Result, error) {
			calls = append(calls, "f2")
			seen = append(seen, got)
			return Result{}, nil
		})

		_, err := Run(d)
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal([]string{"f1", "f2"}))
		Expect(seen).To(HaveLen(2))
		Expect(seen[0].Inbound).To(Equal(seen[1].Inbound))
	})

	It("checks that Run stops at the first failing function and returns its error", func() {
		errX := errors.New("x")
		f2Called := false

		Register(func(Deps) (Result, error) { return Result{}, errX })
		Register(func(Deps) (Result, error) {
			f2Called = true
			return Result{}, nil
		})

		_, err := Run(testDeps())
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, errX)).To(BeTrue())
		Expect(f2Called).To(BeFalse())
	})

	It("checks that Run returns nil when nothing is registered", func() {
		_, err := Run(testDeps())
		Expect(err).NotTo(HaveOccurred())
	})

	It("checks that Run refuses a second Status and returns a zero Result", func() {
		p1 := &models.Communicator{State: "p1"}
		p2 := &models.Communicator{State: "p2"}
		s1 := func(context.Context, int) *models.Communicator { return p1 }
		s2 := func(context.Context, int) *models.Communicator { return p2 }

		a := config.ChildSpec{Name: "a", WorkerType: "test-worker", UserSpec: config.UserSpec{Config: "a: true"}, Enabled: true}
		b := config.ChildSpec{Name: "b", WorkerType: "test-worker", UserSpec: config.UserSpec{Config: "b: true"}, Enabled: true}

		Register(func(Deps) (Result, error) {
			return Result{Children: []config.ChildSpec{a}, Status: s1}, nil
		})
		Register(func(Deps) (Result, error) {
			return Result{Children: []config.ChildSpec{b}, Status: s2}, nil
		})

		result, err := Run(testDeps())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(MatchRegexp(`\b0\b`))
		Expect(err.Error()).To(MatchRegexp(`\b1\b`))
		Expect(result.Children).To(BeNil())
		Expect(result.Status).To(BeNil())
	})

	It("checks that Run's refusal names the two functions that provide a Status, by position", func() {
		p1 := &models.Communicator{State: "p1"}
		p2 := &models.Communicator{State: "p2"}
		s1 := func(context.Context, int) *models.Communicator { return p1 }
		s2 := func(context.Context, int) *models.Communicator { return p2 }

		Register(func(Deps) (Result, error) { return Result{}, nil })
		Register(func(Deps) (Result, error) {
			return Result{Status: s1}, nil
		})
		Register(func(Deps) (Result, error) {
			return Result{Status: s2}, nil
		})

		result, err := Run(testDeps())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(MatchRegexp(`\b1\b`))
		Expect(err.Error()).To(MatchRegexp(`\b2\b`))
		Expect(err.Error()).NotTo(MatchRegexp(`\b0\b`))
		Expect(result.Children).To(BeNil())
		Expect(result.Status).To(BeNil())
	})

	It("checks that Run keeps the one Status when only one function provides it", func() {
		p1 := &models.Communicator{State: "p1"}
		s1 := func(context.Context, int) *models.Communicator { return p1 }

		a := config.ChildSpec{Name: "a", WorkerType: "test-worker", UserSpec: config.UserSpec{Config: "a: true"}, Enabled: true}
		b := config.ChildSpec{Name: "b", WorkerType: "test-worker", UserSpec: config.UserSpec{Config: "b: true"}, Enabled: true}

		Register(func(Deps) (Result, error) {
			return Result{Children: []config.ChildSpec{a}, Status: s1}, nil
		})
		Register(func(Deps) (Result, error) {
			return Result{Children: []config.ChildSpec{b}}, nil
		})

		result, err := Run(testDeps())
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Children).To(Equal([]config.ChildSpec{a, b}))
		Expect(result.Status).NotTo(BeNil())
		Expect(result.Status(context.Background(), 0)).To(Equal(p1))
	})
})
