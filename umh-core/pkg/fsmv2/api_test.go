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
)

// Tests run as part of FSMv2 Dependencies Suite (see dependencies_test.go)

var _ = Describe("Identity", func() {
	Describe("String()", func() {
		It("returns HierarchyPath when available", func() {
			id := fsmv2.Identity{
				ID:            "comm-001",
				WorkerType:    "communicator",
				HierarchyPath: "app/comm-001",
			}
			Expect(id.String()).To(Equal("app/comm-001"))
		})

		It("falls back to ID(Type) for root workers without HierarchyPath", func() {
			id := fsmv2.Identity{
				ID:         "comm-001",
				WorkerType: "communicator",
			}
			Expect(id.String()).To(Equal("comm-001(communicator)"))
		})

		It("returns unknown when identity is empty", func() {
			id := fsmv2.Identity{}
			Expect(id.String()).To(Equal("unknown"))
		})

		It("returns unknown when only ID is set", func() {
			id := fsmv2.Identity{ID: "comm-001"}
			Expect(id.String()).To(Equal("unknown"))
		})

		It("returns unknown when only WorkerType is set", func() {
			id := fsmv2.Identity{WorkerType: "communicator"}
			Expect(id.String()).To(Equal("unknown"))
		})

		It("prefers HierarchyPath over ID(Type) fallback", func() {
			id := fsmv2.Identity{
				ID:            "comm-001",
				Name:          "Communicator Worker",
				WorkerType:    "communicator",
				HierarchyPath: "scenario123(application)/comm-001(communicator)",
			}
			Expect(id.String()).To(Equal("scenario123(application)/comm-001(communicator)"))
		})
	})
})
