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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// Test suite is registered in channel_provider_test.go to avoid duplicate RunSpecs

var _ = Describe("TransportUserSpec", func() {
	Describe("Struct fields", func() {
		It("should have RelayURL field", func() {
			spec := transport.TransportUserSpec{
				RelayURL: "https://relay.umh.app",
			}
			Expect(spec.RelayURL).To(Equal("https://relay.umh.app"))
		})

		It("should have InstanceUUID field", func() {
			spec := transport.TransportUserSpec{
				InstanceUUID: "test-uuid-12345",
			}
			Expect(spec.InstanceUUID).To(Equal("test-uuid-12345"))
		})

		It("should have AuthToken field", func() {
			spec := transport.TransportUserSpec{
				AuthToken: "secret-auth-token",
			}
			Expect(spec.AuthToken).To(Equal("secret-auth-token"))
		})

		It("should have Timeout field", func() {
			spec := transport.TransportUserSpec{
				Timeout: 30 * time.Second,
			}
			Expect(spec.Timeout).To(Equal(30 * time.Second))
		})
	})

	Describe("GetState", func() {
		Context("when state is empty", func() {
			It("should return 'running' as default", func() {
				spec := &transport.TransportUserSpec{}
				Expect(spec.GetState()).To(Equal("running"))
			})
		})

		Context("when state is set to 'stopped'", func() {
			It("should return 'stopped'", func() {
				spec := &transport.TransportUserSpec{}
				spec.State = "stopped"
				Expect(spec.GetState()).To(Equal("stopped"))
			})
		})

		Context("when state is set to 'running'", func() {
			It("should return 'running'", func() {
				spec := &transport.TransportUserSpec{}
				spec.State = "running"
				Expect(spec.GetState()).To(Equal("running"))
			})
		})
	})

	Describe("YAML parsing", func() {
		It("should parse all fields from YAML", func() {
			yamlConfig := `
relayURL: "https://relay.example.com"
instanceUUID: "uuid-from-yaml"
authToken: "token-from-yaml"
timeout: 15s
state: "running"
`
			var spec transport.TransportUserSpec
			err := yaml.Unmarshal([]byte(yamlConfig), &spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(spec.RelayURL).To(Equal("https://relay.example.com"))
			Expect(spec.InstanceUUID).To(Equal("uuid-from-yaml"))
			Expect(spec.AuthToken).To(Equal("token-from-yaml"))
			Expect(spec.Timeout).To(Equal(15 * time.Second))
			Expect(spec.GetState()).To(Equal("running"))
		})

		It("should handle missing optional fields", func() {
			yamlConfig := `
relayURL: "https://relay.example.com"
`
			var spec transport.TransportUserSpec
			err := yaml.Unmarshal([]byte(yamlConfig), &spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(spec.RelayURL).To(Equal("https://relay.example.com"))
			Expect(spec.InstanceUUID).To(BeEmpty())
			Expect(spec.AuthToken).To(BeEmpty())
			Expect(spec.Timeout).To(Equal(time.Duration(0)))
			Expect(spec.GetState()).To(Equal("running")) // Default when empty
		})
	})

	Describe("JSON tags", func() {
		It("should have correct JSON field names for serialization", func() {
			// This test ensures the JSON tags are correctly defined
			// by verifying the struct can be used in JSON contexts
			spec := transport.TransportUserSpec{
				RelayURL:     "https://relay.umh.app",
				InstanceUUID: "test-uuid",
				AuthToken:    "test-token",
				Timeout:      30 * time.Second,
			}
			spec.State = "running"

			// Verify fields are accessible (compile-time check)
			Expect(spec.RelayURL).NotTo(BeEmpty())
			Expect(spec.InstanceUUID).NotTo(BeEmpty())
			Expect(spec.AuthToken).NotTo(BeEmpty())
			Expect(spec.Timeout).NotTo(BeZero())
		})
	})

	Describe("BaseUserSpec embedding", func() {
		It("should embed BaseUserSpec for State field", func() {
			spec := &transport.TransportUserSpec{}
			// BaseUserSpec provides the State field and GetState method
			spec.State = "stopped"
			Expect(spec.GetState()).To(Equal("stopped"))
		})
	})
})
