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

package communicator_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fsmconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// P2-3: communicator RenderChildren 4-field round-trip guard.
//
// All four auth parameters (RelayURL, InstanceUUID, AuthToken, Timeout) must
// survive the full path: CommunicatorConfig → RenderChildren → NewChildSpec →
// UserSpec.Config → ParseUserSpec[TransportUserSpec].
//
// A silent field-swap (e.g. AuthToken: cfg.RelayURL) compiles but produces an
// unauthenticated transport child once the cutover wires it. This test catches
// the field-swap before the cutover.
var _ = Describe("communicator RenderChildren 4-field round-trip (P2-3)", func() {
	var cfg communicator.CommunicatorConfig

	BeforeEach(func() {
		cfg = communicator.CommunicatorConfig{
			RelayURL:     "https://relay.example.com:8080",
			InstanceUUID: "test-uuid-1234",
			AuthToken:    "secret-auth-token-xyz",
			Timeout:      42 * time.Second,
		}
	})

	It("produces exactly one child spec named 'transport'", func() {
		specs, err := communicator.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(1))
		Expect(specs[0].Name).To(Equal("transport"))
		Expect(specs[0].WorkerType).To(Equal("transport"))
	})

	It("round-trips all 4 auth fields to TransportUserSpec (auth round-trip guard)", func() {
		specs, err := communicator.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(1))

		recovered, err := fsmconfig.ParseUserSpec[transport.TransportUserSpec](specs[0].UserSpec)
		Expect(err).ToNot(HaveOccurred())

		Expect(recovered.RelayURL).To(Equal("https://relay.example.com:8080"),
			"RelayURL must not be swapped with another field")
		Expect(recovered.InstanceUUID).To(Equal("test-uuid-1234"),
			"InstanceUUID must survive the round-trip")
		Expect(recovered.AuthToken).To(Equal("secret-auth-token-xyz"),
			"AuthToken missing = unauthenticated transport child after the cutover")
		Expect(recovered.Timeout).To(Equal(42 * time.Second),
			"Timeout must survive the round-trip")
	})

	It("sets Enabled=true when enabled=true", func() {
		specs, err := communicator.RenderChildren(cfg, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs[0].Enabled).To(BeTrue())
	})

	It("sets Enabled=false when enabled=false (stop-state resident-disable guard)", func() {
		specs, err := communicator.RenderChildren(cfg, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs[0].Enabled).To(BeFalse())
	})
})
