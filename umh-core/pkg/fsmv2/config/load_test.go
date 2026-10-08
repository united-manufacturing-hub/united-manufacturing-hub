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

package config_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

type loadHostConfig struct {
	Host string `yaml:"host"`
}

type loadEntryConnection struct {
	Host string          `yaml:"host"`
	Read config.Template `yaml:"read"`
}

type loadEntryConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	Name                string              `yaml:"name"`
	Read                config.Template     `yaml:"read"`
	Connection          loadEntryConnection `yaml:"connection"`
}

type loadInnerConfig struct {
	Read config.Template `yaml:"read"`
}

type loadInlineConfig struct {
	Inner loadInnerConfig `yaml:",inline"`
	Name  string          `yaml:"name"`
}

var _ = Describe("LoadUserSpec", func() {
	It("reports a spec that is not a UserSpec as a plain error, not a config error", func() {
		_, err := config.LoadUserSpec[struct{}]("not a spec")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid spec type"))

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeFalse(),
			"a wrong spec type is a programming error, so it must keep reaching Sentry")
	})

	It("rejects a placeholder in a plain field, naming its YAML path", func() {
		spec := config.UserSpec{Config: "host: \"{{ .x }}\""}

		_, err := config.LoadUserSpec[loadHostConfig](spec)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("host"))
	})

	It("keeps a dollar-sign value untouched", func() {
		spec := config.UserSpec{Config: "host: ${X}"}

		parsed, err := config.LoadUserSpec[loadHostConfig](spec)

		Expect(err).NotTo(HaveOccurred())
		Expect(parsed.Host).To(Equal("${X}"))
	})

	It("returns the zero value for a nil spec", func() {
		parsed, err := config.LoadUserSpec[loadHostConfig](nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(Equal(loadHostConfig{}))
	})

	It("returns the zero value for an empty config", func() {
		spec := config.UserSpec{}

		parsed, err := config.LoadUserSpec[loadHostConfig](spec)

		Expect(err).NotTo(HaveOccurred())
		Expect(parsed).To(Equal(loadHostConfig{}))
	})

	It("wraps a YAML decode error in a ConfigError", func() {
		spec := config.UserSpec{Config: "host: [not, a, string]"}

		_, err := config.LoadUserSpec[loadHostConfig](spec)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("cannot unmarshal"))
	})

	It("checks the user variables under the variables prefix", func() {
		spec := config.UserSpec{
			Config: "host: ok",
			Variables: config.VariableBundle{
				User: map[string]any{"IP": "{{ .x }}"},
			},
		}

		_, err := config.LoadUserSpec[loadHostConfig](spec)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("variables.IP"))
	})
})

var _ = Describe("EntryFields", func() {
	It("removes the variables key and every template key, keeping the rest", func() {
		cfg := loadEntryConfig{}

		fields, err := config.EntryFields("state: running\nname: b1\nread: \"{{ .name }}-x\"\nconnection:\n  host: h1\n  read: \"{{ .connection.host }}\"\nvariables:\n  IP: 10.0.0.1\n", &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(fields).To(Equal(map[string]any{
			"state": "running",
			"name":  "b1",
			"connection": map[string]any{
				"host": "h1",
			},
		}))
	})

	It("honours an inline embed when naming a template key", func() {
		cfg := loadInlineConfig{}

		fields, err := config.EntryFields("name: b1\nread: \"{{ .name }}\"", &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(fields).To(Equal(map[string]any{"name": "b1"}))
	})
})
