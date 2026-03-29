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

var _ = Describe("TransportConfig", func() {
	Describe("Struct fields", func() {
		It("should have RelayURL field", func() {
			cfg := transport.TransportConfig{
				RelayURL: "https://relay.umh.app",
			}
			Expect(cfg.RelayURL).To(Equal("https://relay.umh.app"))
		})

		It("should have InstanceUUID field", func() {
			cfg := transport.TransportConfig{
				InstanceUUID: "test-uuid-12345",
			}
			Expect(cfg.InstanceUUID).To(Equal("test-uuid-12345"))
		})

		It("should have AuthToken field", func() {
			cfg := transport.TransportConfig{
				AuthToken: "secret-auth-token",
			}
			Expect(cfg.AuthToken).To(Equal("secret-auth-token"))
		})

		It("should have Timeout field", func() {
			cfg := transport.TransportConfig{
				Timeout: 30 * time.Second,
			}
			Expect(cfg.Timeout).To(Equal(30 * time.Second))
		})
	})

	Describe("GetState", func() {
		Context("when state is empty", func() {
			It("should return 'running' as default", func() {
				cfg := &transport.TransportConfig{}
				Expect(cfg.GetState()).To(Equal("running"))
			})
		})

		Context("when state is set to 'stopped'", func() {
			It("should return 'stopped'", func() {
				cfg := &transport.TransportConfig{}
				cfg.State = "stopped"
				Expect(cfg.GetState()).To(Equal("stopped"))
			})
		})

		Context("when state is set to 'running'", func() {
			It("should return 'running'", func() {
				cfg := &transport.TransportConfig{}
				cfg.State = "running"
				Expect(cfg.GetState()).To(Equal("running"))
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
			var cfg transport.TransportConfig
			err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.RelayURL).To(Equal("https://relay.example.com"))
			Expect(cfg.InstanceUUID).To(Equal("uuid-from-yaml"))
			Expect(cfg.AuthToken).To(Equal("token-from-yaml"))
			Expect(cfg.Timeout).To(Equal(15 * time.Second))
			Expect(cfg.GetState()).To(Equal("running"))
		})

		It("should handle missing optional fields", func() {
			yamlConfig := `
relayURL: "https://relay.example.com"
`
			var cfg transport.TransportConfig
			err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.RelayURL).To(Equal("https://relay.example.com"))
			Expect(cfg.InstanceUUID).To(BeEmpty())
			Expect(cfg.AuthToken).To(BeEmpty())
			Expect(cfg.Timeout).To(Equal(time.Duration(0)))
			Expect(cfg.GetState()).To(Equal("running")) // Default when empty
		})
	})

	Describe("BaseUserSpec embedding", func() {
		It("should embed BaseUserSpec for State field", func() {
			cfg := &transport.TransportConfig{}
			cfg.State = "stopped"
			Expect(cfg.GetState()).To(Equal("stopped"))
		})
	})
})

var _ = Describe("TransportStatus", func() {
	Describe("HasValidToken", func() {
		It("should return false when JWT token is empty", func() {
			status := transport.TransportStatus{
				JWTToken:  "",
				JWTExpiry: time.Now().Add(1 * time.Hour),
			}
			Expect(status.HasValidToken()).To(BeFalse())
		})

		It("should return false when token is present but expired", func() {
			status := transport.TransportStatus{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(-1 * time.Hour),
			}
			Expect(status.HasValidToken()).To(BeFalse())
		})

		It("should return true when token is present and not expired", func() {
			status := transport.TransportStatus{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(1 * time.Hour),
			}
			Expect(status.HasValidToken()).To(BeTrue())
		})
	})

	Describe("IsTokenExpired", func() {
		It("should return false when expiry is zero", func() {
			status := transport.TransportStatus{
				JWTToken: "some-token",
			}
			Expect(status.IsTokenExpired()).To(BeFalse())
		})

		It("should return true when token expires within refresh buffer (10 min)", func() {
			status := transport.TransportStatus{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(5 * time.Minute),
			}
			Expect(status.IsTokenExpired()).To(BeTrue())
		})

		It("should return false when token expires beyond refresh buffer", func() {
			status := transport.TransportStatus{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(15 * time.Minute),
			}
			Expect(status.IsTokenExpired()).To(BeFalse())
		})

		It("should return true when token already expired", func() {
			status := transport.TransportStatus{
				JWTToken:  "expired-token",
				JWTExpiry: time.Now().Add(-1 * time.Hour),
			}
			Expect(status.IsTokenExpired()).To(BeTrue())
		})
	})
})
