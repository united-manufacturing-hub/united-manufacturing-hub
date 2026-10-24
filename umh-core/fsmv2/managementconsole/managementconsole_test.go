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

package managementconsole

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
)

var _ = Describe("Register and Run", func() {
	// Each spec starts from the open-source state: no start function set.
	BeforeEach(func() {
		start = nil
		DeferCleanup(func() { start = nil })
	})

	It("returns an empty Result when nothing is registered", func() {
		r, err := Run(Deps{})
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Children).To(BeNil())
		Expect(r.Status).To(BeNil())
	})

	It("calls the registered start function with the given Deps", func() {
		child := config.ChildSpec{Name: "x", WorkerType: "y", Enabled: true}
		errStart := errors.New("planted start error")

		var got Deps

		Register(func(d Deps) (Result, error) {
			got = d

			return Result{Children: []config.ChildSpec{child}}, errStart
		})

		r, err := Run(Deps{Config: Config{RelayURL: "https://relay.invalid"}})
		Expect(err).To(MatchError(errStart))
		Expect(r.Children).To(ConsistOf(child))
		Expect(got.Config.RelayURL).To(Equal("https://relay.invalid"))
	})

	It("panics on a second Register", func() {
		Register(func(Deps) (Result, error) { return Result{}, nil })

		Expect(func() {
			Register(func(Deps) (Result, error) { return Result{}, nil })
		}).To(Panic())
	})

	It("panics on Register(nil) and sets nothing", func() {
		Expect(func() { Register(nil) }).To(Panic())
		Expect(start).To(BeNil())
	})
})
