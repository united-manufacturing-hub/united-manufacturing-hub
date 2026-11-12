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

package diagnosis

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Capabilities are startup facts the caller owns; NewEnvironment is the only
// route to an Environment, because a package that cannot touch the unexported
// fields has no other way to build one and therefore cannot call Observe.
var _ = Describe("Environment", func() {

	It("should let a package that cannot touch its fields build an Environment from a set of capabilities", func() {
		env := NewEnvironment("a", "b")

		Expect(env.Has("a")).To(BeTrue(), "a capability the environment was built with must be present")
		Expect(env.Has("b")).To(BeTrue(), "a second capability the environment was built with must be present")
		Expect(env.Has("c")).To(BeFalse(), "a capability the environment was not built with must be absent")
	})

	It("should report nothing present in an Environment built with no capabilities", func() {
		env := NewEnvironment()

		Expect(env.Has("a")).To(BeFalse(), "an empty environment has no capabilities")
		Expect(env.Has("b")).To(BeFalse(), "an empty environment has no capabilities")
	})
})
