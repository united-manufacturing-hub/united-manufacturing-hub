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
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("BuildNames", func() {
	It("places every row", func() {
		names, err := config.BuildNames(config.Entry{
			Fields:    map[string]any{"name": "b1", "connection": map[string]any{"host": "h"}},
			Variables: map[string]any{"SLAVE_ID": "7"},
			Config:    map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}},
			ReadyMade: map[string]any{"location_path": "a.b"},
			Shortcuts: map[string]any{"IP": "h"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(names).To(Equal(map[string]any{
			"name":          "b1",
			"connection":    map[string]any{"host": "h"},
			"variables":     map[string]any{"SLAVE_ID": "7"},
			"config":        map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}},
			"location_path": "a.b",
			"IP":            "h",
		}))
	})

	It("lets a ready-made value replace a field of the same name", func() {
		ready := map[string]any{"0": "acme", "1": "hall"}
		names, err := config.BuildNames(config.Entry{
			Fields:    map[string]any{"location": map[string]any{"ignored": true}},
			ReadyMade: map[string]any{"location": ready},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(names["location"]).To(Equal(ready))
	})

	It("rejects a field named config, naming both sources", func() {
		_, err := config.BuildNames(config.Entry{
			Fields: map[string]any{"config": map[string]any{"host": "db"}},
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("config"))
		Expect(err.Error()).To(ContainSubstring("field"))

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("config"))
	})

	It("rejects a field named variables, naming both sources", func() {
		_, err := config.BuildNames(config.Entry{
			Fields: map[string]any{"variables": map[string]any{"A": 1}},
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("variables"))
		Expect(err.Error()).To(ContainSubstring("field"))

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("variables"))
	})

	It("rejects a ready-made value named like a section, naming both sources", func() {
		for _, section := range []string{"config", "variables"} {
			_, err := config.BuildNames(config.Entry{
				ReadyMade: map[string]any{section: "x"},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(section))
			Expect(err.Error()).To(ContainSubstring("ready-made value"))

			var cfgErr *config.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Field).To(Equal(section))
		}
	})

	It("rejects a shortcut that clashes with a ready-made value, naming both sources", func() {
		_, err := config.BuildNames(config.Entry{
			ReadyMade: map[string]any{"location": map[string]any{"0": "acme"}},
			Shortcuts: map[string]any{"location": "acme"},
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("shortcut"))
		Expect(err.Error()).To(ContainSubstring("ready-made value"))

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("location"))
	})

	It("rejects a shortcut that clashes with a field", func() {
		_, err := config.BuildNames(config.Entry{
			Fields:    map[string]any{"name": "b1"},
			Shortcuts: map[string]any{"name": "other"},
		})

		Expect(err).To(HaveOccurred())

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("name"))
	})

	It("rejects a shortcut named like a section, naming both sources", func() {
		for _, section := range []string{"config", "variables"} {
			_, err := config.BuildNames(config.Entry{
				Shortcuts: map[string]any{section: "x"},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(section))
			Expect(err.Error()).To(ContainSubstring("shortcut"))

			var cfgErr *config.ConfigError
			Expect(errors.As(err, &cfgErr)).To(BeTrue())
			Expect(cfgErr.Field).To(Equal(section))
		}
	})

	It("keeps a variable that is itself a map whole", func() {
		names, err := config.BuildNames(config.Entry{
			Variables: map[string]any{"user": map[string]any{"a": 1}},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(names["variables"]).To(Equal(map[string]any{"user": map[string]any{"a": 1}}))
	})

	It("fills the sections with empty maps when the entry carries none", func() {
		names, err := config.BuildNames(config.Entry{
			Fields: map[string]any{"name": "b1"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(names["variables"]).To(Equal(map[string]any{}))
		Expect(names["config"]).To(Equal(map[string]any{}))
	})

	It("returns names json.Marshal can encode", func() {
		names, err := config.BuildNames(config.Entry{
			Fields: map[string]any{"location": map[any]any{1: "hall"}},
		})

		Expect(err).NotTo(HaveOccurred())
		_, err = json.Marshal(names)
		Expect(err).NotTo(HaveOccurred())
		Expect(names["location"]).To(Equal(map[string]any{"1": "hall"}))
	})

	It("does not mutate its inputs", func() {
		entry := config.Entry{
			Fields:    map[string]any{"name": "b1", "location": map[any]any{1: "hall"}},
			Variables: map[string]any{"SLAVE_ID": "7", "user": map[string]any{"a": 1}},
			Config:    map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}},
			ReadyMade: map[string]any{"location": map[string]any{"0": "acme"}, "location_path": "acme.hall"},
			Shortcuts: map[string]any{"IP": "h"},
		}

		_, err := config.BuildNames(entry)
		Expect(err).NotTo(HaveOccurred())

		Expect(entry.Fields).To(Equal(map[string]any{"name": "b1", "location": map[any]any{1: "hall"}}))
		Expect(entry.Variables).To(Equal(map[string]any{"SLAVE_ID": "7", "user": map[string]any{"a": 1}}))
		Expect(entry.Config).To(Equal(map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}}))
		Expect(entry.ReadyMade).To(Equal(map[string]any{"location": map[string]any{"0": "acme"}, "location_path": "acme.hall"}))
		Expect(entry.Shortcuts).To(Equal(map[string]any{"IP": "h"}))
	})
})

var _ = Describe("VariableShortcuts", func() {
	It("makes one top-level name per variable", func() {
		names := config.VariableShortcuts(map[string]any{"A": 1})

		Expect(names).To(Equal(map[string]any{"A": 1}))
	})

	It("returns a shallow copy, not the input map", func() {
		variables := map[string]any{"A": 1}
		names := config.VariableShortcuts(variables)
		variables["B"] = 2

		Expect(names).To(Equal(map[string]any{"A": 1}))
	})
})

var _ = Describe("HistorianShortcut", func() {
	It("passes the historian config through when present", func() {
		historian := map[string]any{"timescale": map[string]any{"host": "db"}}
		names := config.HistorianShortcut(map[string]any{"historian": historian})

		Expect(names).To(Equal(map[string]any{"historian": historian}))
	})

	It("returns an empty map when the historian is absent", func() {
		names := config.HistorianShortcut(map[string]any{})

		Expect(names).To(Equal(map[string]any{}))
	})
})

// namesHookConfig is a config type whose name methods place fixed values, so a
// test can check BuildWorkerNames carries their results into the names.
type namesHookConfig struct{}

func (namesHookConfig) ReadyMade(map[string]any) (map[string]any, error) {
	return map[string]any{"location_path": "acme.hall"}, nil
}

func (namesHookConfig) Shortcuts(map[string]any) (map[string]any, error) {
	return map[string]any{"IP": "10.0.0.5"}, nil
}

// pointerHookConfig implements ReadyMade on its pointer, so a test can check
// BuildWorkerNames finds the method behind the pointer too.
type pointerHookConfig struct{}

func (*pointerHookConfig) ReadyMade(map[string]any) (map[string]any, error) {
	return map[string]any{"ready": "ready-value"}, nil
}

// errReadyConfig is a config type whose ReadyMade returns a plain error.
type errReadyConfig struct{}

func (errReadyConfig) ReadyMade(map[string]any) (map[string]any, error) {
	return nil, errors.New("ready-made failed")
}

// errShortcutsConfig is a config type whose Shortcuts returns a plain error.
type errShortcutsConfig struct{}

func (errShortcutsConfig) Shortcuts(map[string]any) (map[string]any, error) {
	return nil, errors.New("shortcuts failed")
}

var _ = Describe("BuildWorkerNames", func() {
	It("places the names both methods supply", func() {
		names, err := config.BuildWorkerNames(namesHookConfig{},
			map[string]any{"name": "b1"},
			map[string]any{"SLAVE_ID": "7"},
			map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}},
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(names["name"]).To(Equal("b1"))
		Expect(names["location_path"]).To(Equal("acme.hall"))
		Expect(names["IP"]).To(Equal("10.0.0.5"))
	})

	It("finds a method implemented on the pointer", func() {
		names, err := config.BuildWorkerNames(pointerHookConfig{},
			map[string]any{"name": "b1"},
			map[string]any{},
			map[string]any{},
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(names["ready"]).To(Equal("ready-value"))
	})

	It("wraps a plain ReadyMade error as a *ConfigError", func() {
		_, err := config.BuildWorkerNames(errReadyConfig{},
			map[string]any{"name": "b1"},
			map[string]any{},
			map[string]any{},
		)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue(),
			"a plain error from a name method must be wrapped as a *config.ConfigError, got %T", err)
		Expect(err).To(MatchError(ContainSubstring("ready-made failed")))
	})

	It("wraps a plain Shortcuts error as a *ConfigError", func() {
		_, err := config.BuildWorkerNames(errShortcutsConfig{},
			map[string]any{"name": "b1"},
			map[string]any{},
			map[string]any{},
		)

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue(),
			"a plain error from a name method must be wrapped as a *config.ConfigError, got %T", err)
		Expect(err).To(MatchError(ContainSubstring("shortcuts failed")))
	})

	It("builds names from the fields, variables and config alone for a config type with neither method", func() {
		names, err := config.BuildWorkerNames(struct{}{},
			map[string]any{"name": "b1"},
			map[string]any{"SLAVE_ID": "7"},
			map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}},
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(names).To(Equal(map[string]any{
			"name":      "b1",
			"variables": map[string]any{"SLAVE_ID": "7"},
			"config":    map[string]any{"historian": map[string]any{"timescale": map[string]any{"host": "db"}}},
		}))
	})
})
