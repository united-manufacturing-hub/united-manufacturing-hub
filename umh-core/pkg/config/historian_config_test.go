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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HistorianConfig", func() {
	// validConfig returns a minimal config that passes Validate.
	validConfig := func() HistorianConfig {
		return HistorianConfig{
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
			Expect(validConfig().Validate()).To(Succeed())
		})

		It("should fail when host is missing", func() {
			cfg := validConfig()
			cfg.Host = ""

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field host"))
		})

		It("should fail when password is missing", func() {
			cfg := validConfig()
			cfg.Password = ""

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field password"))
		})

		It("should report the missing host before the missing password", func() {
			cfg := HistorianConfig{}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("host"))
		})

		It("should pass when sslmode is left empty (default applied later)", func() {
			cfg := validConfig()
			cfg.SSLMode = ""

			Expect(cfg.Validate()).To(Succeed())
		})

		It("should pass with a valid explicit sslmode", func() {
			cfg := validConfig()
			cfg.SSLMode = HistorianSSLModeVerifyFull

			Expect(cfg.Validate()).To(Succeed())
		})

		It("should fail with an invalid sslmode", func() {
			cfg := validConfig()
			cfg.SSLMode = "bogus"

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid sslmode"))
			Expect(err.Error()).To(ContainSubstring("bogus"))
		})
	})

	Describe("WithDefaults", func() {
		It("should fill all optional fields when unset", func() {
			cfg := validConfig().WithDefaults()

			Expect(cfg.Port).To(Equal(uint16(5432)))
			Expect(cfg.Database).To(Equal("umh"))
			Expect(cfg.Username).To(Equal("umh_owner"))
			Expect(cfg.SSLMode).To(Equal(HistorianSSLModeRequire))
		})

		It("should leave required fields unchanged", func() {
			cfg := validConfig().WithDefaults()

			Expect(cfg.Host).To(Equal("timescale.example.com"))
			Expect(cfg.Password).To(Equal("secret"))
		})

		It("should not override values that are already set", func() {
			cfg := HistorianConfig{
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
			cfg := validConfig()
			_ = cfg.WithDefaults()

			Expect(cfg.Port).To(Equal(uint16(0)))
			Expect(cfg.Database).To(BeEmpty())
			Expect(cfg.Username).To(BeEmpty())
			Expect(cfg.SSLMode).To(BeEmpty())
		})
	})

	Describe("FullConfig.Clone", func() {
		It("should deep-copy the historian section", func() {
			original := FullConfig{
				Historian: &HistorianConfig{
					Host:     "orig-host",
					Password: "orig-pass",
					Port:     5432,
				},
			}

			clone := original.Clone()
			Expect(clone.Historian).NotTo(BeNil())
			Expect(clone.Historian).NotTo(BeIdenticalTo(original.Historian))
			Expect(*clone.Historian).To(Equal(*original.Historian))

			// Mutating the clone must not affect the original.
			clone.Historian.Host = "changed"
			Expect(original.Historian.Host).To(Equal("orig-host"))
		})

		It("should keep a nil historian nil", func() {
			original := FullConfig{}
			clone := original.Clone()
			Expect(clone.Historian).To(BeNil())
		})
	})
})
