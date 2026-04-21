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

package transport_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

var _ = Describe("Transport ChildDeps singleton", func() {
	BeforeEach(func() {
		transport.ClearChildDeps()
	})

	AfterEach(func() {
		transport.ClearChildDeps()
		transport.ClearChannelProvider()
	})

	Context("when nothing is published", func() {
		It("returns nil", func() {
			Expect(transport.ChildDeps()).To(BeNil())
		})
	})

	Context("when SetChildDeps is called", func() {
		It("round-trips the published value", func() {
			var d *transport.TransportDependencies
			d = &transport.TransportDependencies{}

			transport.SetChildDeps(d)
			Expect(transport.ChildDeps()).To(BeIdenticalTo(d))
		})

		It("is cleared by ClearChildDeps", func() {
			transport.SetChildDeps(&transport.TransportDependencies{})
			Expect(transport.ChildDeps()).NotTo(BeNil())

			transport.ClearChildDeps()
			Expect(transport.ChildDeps()).To(BeNil())
		})
	})

	Context("after the transport worker is instantiated via the factory", func() {
		BeforeEach(func() {
			transport.SetChannelProvider(newTestChannelProvider())
		})

		It("ChildDeps returns the worker's deps", func() {
			identity := deps.Identity{ID: "factory-transport", Name: "Factory Transport", WorkerType: "transport"}
			logger := deps.NewNopFSMLogger()

			w, err := factory.NewWorkerByType("transport", identity, logger, nil, map[string]any{})
			Expect(err).ToNot(HaveOccurred())
			Expect(w).NotTo(BeNil())

			tw, ok := w.(*transport.TransportWorker)
			Expect(ok).To(BeTrue())

			published := transport.ChildDeps()
			Expect(published).NotTo(BeNil())
			Expect(published).To(BeIdenticalTo(tw.GetDependenciesAny()))
		})
	})
})
