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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

type plainConnection struct {
	Host string `yaml:"host"`
}

type plainOuter struct {
	Connection plainConnection `yaml:"connection"`
}

type plainTemplateField struct {
	Read config.Template `yaml:"read"`
}

type plainDollars struct {
	Password string `yaml:"password"`
}

type plainExtraMap struct {
	Extra map[string]string `yaml:"extra"`
}

type plainHosts struct {
	Hosts []string `yaml:"hosts"`
}

type plainHostEntry struct {
	Host string `yaml:"host"`
}

type plainHostEntries struct {
	Hosts []plainHostEntry `yaml:"hosts"`
}

type plainVars struct {
	Vars map[string]any `yaml:"vars"`
}

type plainBase struct {
	State string `yaml:"state"`
}

type plainInline struct {
	Base plainBase `yaml:",inline"`
}

type plainMode string

type plainModeHolder struct {
	Mode plainMode `yaml:"mode"`
}

type plainUnexported struct {
	hidden string
	Host   string `yaml:"host"`
}

type plainHiddenTag struct {
	Host  string `yaml:"host"`
	Value string `yaml:"-"`
}

type plainSpecHolder struct {
	Spec config.UserSpec `yaml:"spec"`
}

type plainTimes struct {
	When    time.Time     `yaml:"when"`
	Timeout time.Duration `yaml:"timeout"`
}

type plainTwoBad struct {
	First  string `yaml:"first"`
	Second string `yaml:"second"`
}

type plainPtrs struct {
	Host *string          `yaml:"host"`
	Read *config.Template `yaml:"read"`
}

type plainPair struct {
	Pair [2]string `yaml:"pair"`
}

type plainSelfDecoded struct {
	Body string `yaml:"body"`
}

func (s *plainSelfDecoded) UnmarshalYAML(value *yaml.Node) error {
	return value.Decode(&s.Body)
}

type plainSelfDecodedHolder struct {
	Decoded plainSelfDecoded `yaml:"decoded"`
}

var _ = Describe("CheckPlainValues", func() {
	It("rejects a placeholder in a plain field, naming its YAML path", func() {
		err := config.CheckPlainValues(plainOuter{
			Connection: plainConnection{Host: "{{ .x }}"},
		}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("connection.host"))
		Expect(err.Error()).To(Equal("connection.host: placeholders are only allowed in template fields"))
	})

	It("allows the same value in a Template field", func() {
		err := config.CheckPlainValues(plainTemplateField{Read: "{{ .x }}"}, nil)

		Expect(err).NotTo(HaveOccurred())
	})

	It("allows dollar-sign values in a plain field", func() {
		err := config.CheckPlainValues(plainDollars{
			Password: `${DB_PASS} and ${! meta("x") }`,
		}, nil)

		Expect(err).NotTo(HaveOccurred())
	})

	It("names a map value by its key", func() {
		err := config.CheckPlainValues(plainExtraMap{
			Extra: map[string]string{"k": "{{ .x }}"},
		}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("extra.k"))
	})

	It("names a list element by its index", func() {
		err := config.CheckPlainValues(plainHosts{
			Hosts: []string{"ok", "{{ .x }}"},
		}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("hosts.1"))
	})

	It("names a list element by its index in the same form as RenderTemplates", func() {
		err := config.CheckPlainValues(plainHostEntries{
			Hosts: []plainHostEntry{{Host: "ok"}, {Host: "{{ .x }}"}},
		}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("hosts.1.host"))
	})

	It("walks a nested any held in a map", func() {
		err := config.CheckPlainValues(plainVars{
			Vars: map[string]any{
				"L": []any{map[string]any{"a": "{{ .x }}"}},
			},
		}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("vars.L.0.a"))
	})

	It("adds no path segment for an inline-embedded struct", func() {
		err := config.CheckPlainValues(plainInline{
			Base: plainBase{State: "{{ .x }}"},
		}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("state"))
	})

	It("detects a named string type", func() {
		err := config.CheckPlainValues(plainModeHolder{Mode: "{{ .x }}"}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("mode"))
	})

	It("skips unexported fields and fields tagged yaml:\"-\"", func() {
		err := config.CheckPlainValues(plainUnexported{hidden: "{{ .x }}", Host: "ok"}, nil)
		Expect(err).NotTo(HaveOccurred())

		err = config.CheckPlainValues(plainHiddenTag{Host: "ok", Value: "{{ .x }}"}, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("treats a UserSpec as a leaf", func() {
		err := config.CheckPlainValues(plainSpecHolder{
			Spec: config.UserSpec{Config: "{{ .x }}"},
		}, nil)

		Expect(err).NotTo(HaveOccurred())
	})

	It("checks the variables map under a variables prefix", func() {
		err := config.CheckPlainValues(plainOuter{
			Connection: plainConnection{Host: "ok"},
		}, map[string]any{"IP": "{{ .x }}"})

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("variables.IP"))
	})

	It("treats time.Time and time.Duration as leaves", func() {
		err := config.CheckPlainValues(plainTimes{
			When:    time.Now(),
			Timeout: 5 * time.Second,
		}, nil)

		Expect(err).NotTo(HaveOccurred())
	})

	It("reports the first bad field in declaration order on every run", func() {
		for range 20 {
			err := config.CheckPlainValues(plainTwoBad{
				First:  "{{ .a }}",
				Second: "{{ .b }}",
			}, nil)

			var cfgErr *config.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Field).To(Equal("first"))
		}
	})

	It("reports the first bad variables key in sorted order on every run", func() {
		for range 20 {
			err := config.CheckPlainValues(plainOuter{
				Connection: plainConnection{Host: "ok"},
			}, map[string]any{"IP": "{{ .x }}", "PORT": "{{ .y }}"})

			var cfgErr *config.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Field).To(Equal("variables.IP"))
		}
	})

	It("walks through a non-nil pointer field and names the inner field", func() {
		host := "{{ .x }}"
		err := config.CheckPlainValues(plainPtrs{Host: &host}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("host"))
	})

	It("skips nil pointer fields and a nil value", func() {
		Expect(config.CheckPlainValues(plainPtrs{}, nil)).NotTo(HaveOccurred())

		var outer *plainOuter
		Expect(config.CheckPlainValues(outer, nil)).NotTo(HaveOccurred())
		Expect(config.CheckPlainValues(nil, nil)).NotTo(HaveOccurred())
	})

	It("names an array element by its index", func() {
		err := config.CheckPlainValues(plainPair{Pair: [2]string{"ok", "{{ .x }}"}}, nil)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("pair.1"))
	})

	It("treats a type with its own unmarshal method as a leaf", func() {
		err := config.CheckPlainValues(plainSelfDecodedHolder{
			Decoded: plainSelfDecoded{Body: "{{ .x }}"},
		}, nil)

		Expect(err).NotTo(HaveOccurred())
	})
})
