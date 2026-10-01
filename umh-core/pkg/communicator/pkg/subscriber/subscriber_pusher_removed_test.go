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

package subscriber_test

import (
	"os"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
)

// Structural pins for the write-only Pusher's deletion from the subscriber.
// The behavioral contract is the FSMv2 status-delivery e2e guard; these are
// fail-fast tokens. Caveat: the reflection check matches the parameter type by
// package-qualified string, so a differently-named pusher type would evade it.
var _ = Describe("The write-only Pusher is gone from the subscriber", func() {
	It("does not take a *push.Pusher parameter in any position", func() {
		sig := reflect.TypeOf(subscriber.NewHandler)
		for i := range sig.NumIn() {
			Expect(sig.In(i).String()).NotTo(Equal("*push.Pusher"),
				"NewHandler parameter %d must not be a *push.Pusher", i)
		}
	})

	It("has no legacy s.pusher.Push send path left in subscribers.go", func() {
		subscribersSrc, err := os.ReadFile("subscribers.go")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(subscribersSrc)).NotTo(ContainSubstring("s.pusher.Push"),
			"notify() must no longer have the legacy s.pusher.Push send path")
	})

	It("has no c.Pusher == nil guards left in communication_state.go", func() {
		commStateSrc, err := os.ReadFile("../../communication_state/communication_state.go")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(commStateSrc)).NotTo(ContainSubstring("c.Pusher == nil"),
			"communication_state must have no c.Pusher == nil guards")
	})
})
