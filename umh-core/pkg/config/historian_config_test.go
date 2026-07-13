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

package config

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TimescaleConfig", func() {
	// validTimescale returns a minimal timescale config that passes Validate.
	validTimescale := func() TimescaleConfig {
		return TimescaleConfig{
			Host:     "timescale.example.com",
			Password: "secret",
		}
	}

	Describe("HistorianSSLMode.IsValid", func() {
		It("should accept the three documented modes", func() {
			Expect(HistorianSSLModeRequire.IsValid()).To(BeTrue())
			Expect(HistorianSSLModeDisable.IsValid()).To(BeTrue())
			Expect(HistorianSSLModeVerifyFull.IsValid()).To(BeTrue())
		})

		It("should reject the empty string and unknown modes", func() {
			Expect(HistorianSSLMode("").IsValid()).To(BeFalse())
			Expect(HistorianSSLMode("verify-ca").IsValid()).To(BeFalse())
			Expect(HistorianSSLMode("REQUIRE").IsValid()).To(BeFalse())
		})
	})

	Describe("Validate", func() {
		It("should pass with all required fields set", func() {
			Expect(validTimescale().Validate()).To(Succeed())
		})

		It("should fail when host is missing", func() {
			cfg := validTimescale()
			cfg.Host = ""

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field host"))
		})

		It("should fail when password is missing", func() {
			cfg := validTimescale()
			cfg.Password = ""

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field password"))
		})

		It("should report the missing host before the missing password", func() {
			cfg := TimescaleConfig{}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("host"))
		})

		It("should pass when sslmode is left empty (default applied later)", func() {
			cfg := validTimescale()
			cfg.SSLMode = ""

			Expect(cfg.Validate()).To(Succeed())
		})

		It("should pass with a valid explicit sslmode", func() {
			cfg := validTimescale()
			cfg.SSLMode = HistorianSSLModeVerifyFull

			Expect(cfg.Validate()).To(Succeed())
		})

		It("should fail with an invalid sslmode", func() {
			cfg := validTimescale()
			cfg.SSLMode = "bogus"

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid sslmode"))
			Expect(err.Error()).To(ContainSubstring("bogus"))
		})

		It("should pass when certificates are paired with verify-full", func() {
			cfg := validTimescale()
			cfg.SSLRootCert = "/certs/ca.pem"
			cfg.SSLMode = HistorianSSLModeVerifyFull

			Expect(cfg.Validate()).To(Succeed())
		})

		It("should reject a CA certificate under sslmode require", func() {
			cfg := validTimescale()
			cfg.SSLRootCert = "/certs/ca.pem"
			cfg.SSLMode = HistorianSSLModeRequire

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verify-full"))
		})

		It("should reject certificates when sslmode is left unset (would default to require)", func() {
			cfg := validTimescale()
			cfg.SSLRootCert = "/certs/ca.pem"
			cfg.SSLMode = ""

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verify-full"))
		})

		It("should reject a client certificate under sslmode disable", func() {
			cfg := validTimescale()
			cfg.SSLCert = "/certs/client.pem"
			cfg.SSLKey = "/certs/client.key"
			cfg.SSLMode = HistorianSSLModeDisable

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verify-full"))
		})
	})

	Describe("ValidateForUpdate", func() {
		It("should pass when the password is missing (kept unchanged on edit)", func() {
			cfg := TimescaleConfig{Host: "timescale.example.com"}
			Expect(cfg.ValidateForUpdate()).To(Succeed())
		})

		It("should still fail when host is missing", func() {
			cfg := TimescaleConfig{Password: "secret"}

			err := cfg.ValidateForUpdate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field host"))
		})

		It("should still reject an invalid sslmode", func() {
			cfg := TimescaleConfig{Host: "timescale.example.com", SSLMode: "bogus"}

			err := cfg.ValidateForUpdate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid sslmode"))
		})

		It("should reject certificates that are not paired with verify-full", func() {
			cfg := TimescaleConfig{
				Host:        "timescale.example.com",
				SSLRootCert: "/certs/ca.pem",
				SSLMode:     HistorianSSLModeRequire,
			}

			err := cfg.ValidateForUpdate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verify-full"))
		})

		It("should accept certificates paired with verify-full", func() {
			cfg := TimescaleConfig{
				Host:        "timescale.example.com",
				SSLRootCert: "/certs/ca.pem",
				SSLMode:     HistorianSSLModeVerifyFull,
			}

			Expect(cfg.ValidateForUpdate()).To(Succeed())
		})
	})

	Describe("WithDefaults", func() {
		It("should fill all optional fields when unset", func() {
			cfg := validTimescale().WithDefaults()

			Expect(cfg.Port).To(Equal(uint16(5432)))
			Expect(cfg.Database).To(Equal("umh"))
			Expect(cfg.Username).To(Equal("umh_owner"))
			Expect(cfg.SSLMode).To(Equal(HistorianSSLModeRequire))
		})

		It("should leave required fields unchanged", func() {
			cfg := validTimescale().WithDefaults()

			Expect(cfg.Host).To(Equal("timescale.example.com"))
			Expect(cfg.Password).To(Equal("secret"))
		})

		It("should not override values that are already set", func() {
			cfg := TimescaleConfig{
				Host:     "h",
				Password: "p",
				Port:     6543,
				Database: "custom_db",
				Username: "custom_user",
				SSLMode:  HistorianSSLModeDisable,
			}

			out := cfg.WithDefaults()
			Expect(out.Port).To(Equal(uint16(6543)))
			Expect(out.Database).To(Equal("custom_db"))
			Expect(out.Username).To(Equal("custom_user"))
			Expect(out.SSLMode).To(Equal(HistorianSSLModeDisable))
		})

		It("should not mutate the receiver", func() {
			cfg := validTimescale()
			_ = cfg.WithDefaults()

			Expect(cfg.Port).To(Equal(uint16(0)))
			Expect(cfg.Database).To(BeEmpty())
			Expect(cfg.Username).To(BeEmpty())
			Expect(cfg.SSLMode).To(BeEmpty())
		})
	})

	Describe("String", func() {
		It("should mask the password but keep the other fields", func() {
			t := TimescaleConfig{
				Host:     "timescale.example.com",
				Password: "super-secret",
				Database: "umh",
				Username: "umh_owner",
				Port:     5432,
			}

			s := t.String()
			Expect(s).NotTo(ContainSubstring("super-secret"))
			Expect(s).To(ContainSubstring("[REDACTED]"))
			Expect(s).To(ContainSubstring("timescale.example.com"))
			Expect(s).To(ContainSubstring("umh_owner"))
		})

		It("should not add a redaction marker when no password is set", func() {
			t := TimescaleConfig{Host: "timescale.example.com"}
			Expect(t.String()).NotTo(ContainSubstring("[REDACTED]"))
		})

		It("should be picked up by a %v format verb", func() {
			t := TimescaleConfig{Host: "h", Password: "super-secret"}
			Expect(fmt.Sprintf("%v", t)).NotTo(ContainSubstring("super-secret"))
			Expect(fmt.Sprintf("%v", &t)).NotTo(ContainSubstring("super-secret"))
		})

		It("should leave the receiver's password untouched", func() {
			t := TimescaleConfig{Host: "h", Password: "super-secret"}
			_ = t.String()
			Expect(t.Password).To(Equal("super-secret"))
		})
	})
})

var _ = Describe("HistorianConfig", func() {
	validTimescale := func() TimescaleConfig {
		return TimescaleConfig{Host: "timescale.example.com", Password: "secret"}
	}

	Describe("Validate", func() {
		It("should pass when the timescale section is valid", func() {
			h := HistorianConfig{Timescale: validTimescale()}
			Expect(h.Validate()).To(Succeed())
		})

		It("should fail when no timescale section is present", func() {
			h := HistorianConfig{}

			err := h.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timescale"))
		})

		It("should surface an invalid timescale section", func() {
			h := HistorianConfig{Timescale: TimescaleConfig{Host: "h"}}

			err := h.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field password"))
		})
	})

	Describe("ValidateForUpdate", func() {
		It("should pass without a password when the timescale section is present", func() {
			h := HistorianConfig{Timescale: TimescaleConfig{Host: "timescale.example.com"}}
			Expect(h.ValidateForUpdate()).To(Succeed())
		})

		It("should fail when no timescale section is present", func() {
			err := HistorianConfig{}.ValidateForUpdate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timescale"))
		})
	})

	Describe("WithDefaults", func() {
		It("should apply defaults to the timescale section", func() {
			h := HistorianConfig{Timescale: validTimescale()}.WithDefaults()

			Expect(h.Timescale.Port).To(Equal(uint16(5432)))
			Expect(h.Timescale.Database).To(Equal("umh"))
		})

		It("should not mutate the receiver", func() {
			h := HistorianConfig{Timescale: validTimescale()}

			_ = h.WithDefaults()
			Expect(h.Timescale.Port).To(Equal(uint16(0)))
		})

		It("should tolerate an empty timescale section", func() {
			Expect(func() { _ = HistorianConfig{}.WithDefaults() }).NotTo(Panic())
		})
	})

	Describe("logging", func() {
		It("should mask the nested timescale password under %v", func() {
			h := HistorianConfig{Timescale: TimescaleConfig{Host: "h", Password: "super-secret"}}
			Expect(fmt.Sprintf("%v", h)).NotTo(ContainSubstring("super-secret"))
		})
	})

	Describe("FullConfig.Clone", func() {
		It("should deep-copy the historian section", func() {
			original := FullConfig{
				Historian: &HistorianConfig{
					Timescale: TimescaleConfig{
						Host:     "orig-host",
						Password: "orig-pass",
						Port:     5432,
					},
				},
			}

			clone := original.Clone()
			Expect(clone.Historian).NotTo(BeNil())
			Expect(clone.Historian).NotTo(BeIdenticalTo(original.Historian))
			Expect(clone.Historian.Timescale).To(Equal(original.Historian.Timescale))

			// Mutating the clone must not affect the original.
			clone.Historian.Timescale.Host = "changed"
			Expect(original.Historian.Timescale.Host).To(Equal("orig-host"))
		})

		It("should keep a nil historian nil", func() {
			original := FullConfig{}
			clone := original.Clone()
			Expect(clone.Historian).To(BeNil())
		})
	})
})
