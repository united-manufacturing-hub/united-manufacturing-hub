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

package generator_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"go.uber.org/zap"
)

var _ = Describe("defaultContainer CPU State wire contract", func() {
	// Verifies FIX 1: defaultContainer() — reached on the nil-snapshot and
	// error paths that flow to MC — must set State="healthy" on models.CPU.
	// State has no omitempty, so without the fix the zero-value "" marshals as
	// "state":"" — a third value outside the {healthy,degraded} contract. The
	// "status unknown" default is healthy (blind-but-quiet = healthy, never a
	// distinct unknown state).
	log := zap.NewNop().Sugar()

	It("marshals state=\"healthy\" (never the zero-value \"\") for a nil snapshot", func() {
		c := generator.ContainerFromSnapshot(nil, log)
		Expect(c.CPU).NotTo(BeNil(), "CPU is always non-nil from defaultContainer")
		Expect(c.CPU.State).To(Equal("healthy"),
			"State must be healthy (never the zero-value \"\")")

		b, err := json.Marshal(c.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(b).To(ContainSubstring(`"state":"healthy"`),
			"the JSON wire must always carry state=healthy (no omitempty)")
		Expect(b).NotTo(ContainSubstring(`"state":""`),
			"the zero-value \"\" must never appear on the wire")
	})

	It("marshals state=\"healthy\" for an error snapshot (nil observed state)", func() {
		// A non-nil snapshot whose LastObservedState is nil fails the
		// type-assertion in buildContainer, which returns an error; the
		// caller falls back to defaultContainer(). The State contract still
		// applies on that path.
		inst := &fsm.FSMInstanceSnapshot{LastObservedState: nil}
		c := generator.ContainerFromSnapshot(inst, log)
		Expect(c.CPU).NotTo(BeNil())
		Expect(c.CPU.State).To(Equal("healthy"))

		b, err := json.Marshal(c.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(b).To(ContainSubstring(`"state":"healthy"`))
	})
})
