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
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

type templateFields struct {
	Name  string          `yaml:"name"`
	Read  config.Template `yaml:"read"`
	Write config.Template `yaml:"write"`
}

type templateInner struct {
	Read config.Template `yaml:"read"`
}

type templateOuterInline struct {
	Inner templateInner `yaml:",inline"`
}

type templateHolder struct {
	Value config.Template `yaml:"value" json:"value"`
}

type templatePtrOuter struct {
	Inner *templateInner `yaml:"inner"`
}

type templateSliceOuter struct {
	Items []templateInner `yaml:"items"`
}

type templatePtrField struct {
	Read *config.Template `yaml:"read"`
}

type templateMapOuter struct {
	Items map[string]templateInner `yaml:"items"`
}

type templateTaggedOuter struct {
	Inner templateInner `yaml:"inner"`
}

type templateHidden struct {
	Value config.Template `yaml:"-"`
}

type templateUnexported struct {
	hidden string
	Read   config.Template `yaml:"read"`
}

var _ = Describe("Template", func() {
	It("renders a marked field with a simple name", func() {
		out, err := config.Template(`{{ .a }}`).Render("read", map[string]any{"a": "x"})

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("x"))
	})

	It("wraps a missing-key error in a ConfigError naming the field", func() {
		out, err := config.Template(`{{ .b }}`).Render("read", map[string]any{"a": "x"})

		Expect(out).To(Equal(""))
		Expect(err).To(HaveOccurred())
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("read"))
		Expect(err.Error()).To(ContainSubstring("b"))
	})

	It("wraps a parse error keeping Go's message", func() {
		out, err := config.Template(`{{ .location.0 }}`).Render("read", map[string]any{})

		Expect(out).To(Equal(""))
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("read"))
		Expect(err.Error()).To(ContainSubstring(`unexpected ".0" in operand`))
	})

	It("errors when an action prints nil", func() {
		_, err := config.Template(`{{ .a }}`).Render("read", map[string]any{"a": nil})

		Expect(err).To(HaveOccurred())
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
	})

	It("errors when an action prints a map or a list directly", func() {
		_, err := config.Template(`{{ .m }}`).Render("read", map[string]any{
			"m": map[string]any{"a": 1},
		})
		Expect(err).To(HaveOccurred())

		_, err = config.Template(`{{ .l }}`).Render("read", map[string]any{
			"l": []any{1},
		})
		Expect(err).To(HaveOccurred())
	})

	It("keeps range, index and if working", func() {
		out, err := config.Template(`{{ range .l }}{{ . }},{{ end }}`).Render("read", map[string]any{
			"l": []any{"a", "b"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("a,b,"))

		out, err = config.Template(`{{ index .m "k" }}`).Render("read", map[string]any{
			"m": map[string]any{"k": "v"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("v"))

		out, err = config.Template(`{{ if .m }}yes{{ end }}`).Render("read", map[string]any{
			"m": map[string]any{"k": "v"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("yes"))
	})

	It("renders whole numbers from a JSON round trip as integers", func() {
		names := map[string]any{"BAUD": int64(1000000)}
		encoded, err := json.Marshal(names)
		Expect(err).NotTo(HaveOccurred())
		var round map[string]any
		err = json.Unmarshal(encoded, &round)
		Expect(err).NotTo(HaveOccurred())

		out, err := config.Template(`{{ .BAUD }}`).Render("read", round)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("1000000"))

		out, err = config.Template(`{{ .f }}`).Render("read", map[string]any{"f": 1.5})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("1.5"))

		nested := map[string]any{"L": []any{map[string]any{"n": float64(1000000)}}}
		encoded, err = json.Marshal(nested)
		Expect(err).NotTo(HaveOccurred())
		var nestedRound map[string]any
		err = json.Unmarshal(encoded, &nestedRound)
		Expect(err).NotTo(HaveOccurred())

		out, err = config.Template(`{{ range .L }}{{ .n }}{{ end }}`).Render("read", nestedRound)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("1000000"))
	})

	It("stringifies map keys so index works on yaml.v3 maps", func() {
		out, err := config.Template(`{{ index .loc "1" }}`).Render("read", map[string]any{
			"loc": map[any]any{1: "hall"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("hall"))
	})

	It("passes benthos-style text outside actions through unchanged", func() {
		in := `${VAR} and ${! meta("x") }`

		out, err := config.Template(in).Render("read", map[string]any{})

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(in))
	})

	It("does not mutate the names", func() {
		makeNames := func() map[string]any {
			return map[string]any{
				"a":   "x",
				"f":   float64(42),
				"m":   map[string]any{"k": "v", "n": float64(7)},
				"loc": map[any]any{1: "hall"},
				"l":   []any{float64(3), "s"},
			}
		}
		names := makeNames()
		before := makeNames()

		_, err := config.Template(`{{ .a }}{{ if .m }}yes{{ end }}{{ index .loc "1" }}{{ range .l }}{{ . }},{{ end }}`).Render("read", names)

		Expect(err).NotTo(HaveOccurred())
		Expect(names).To(Equal(before))
	})

	It("renders all Template fields or none", func() {
		names := map[string]any{"a": "x"}

		s := templateFields{
			Name:  "{{ .a }}",
			Read:  `{{ .a }}`,
			Write: `{{ .missing }}`,
		}
		err := config.RenderTemplates(&s, names)
		Expect(err).To(HaveOccurred())
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("write"))
		Expect(s.Read).To(Equal(config.Template(`{{ .a }}`)))
		Expect(s.Write).To(Equal(config.Template(`{{ .missing }}`)))

		ok := templateFields{
			Name:  "{{ .a }}",
			Read:  `{{ .a }}`,
			Write: `{{ .a }}`,
		}
		err = config.RenderTemplates(&ok, names)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok.Read).To(Equal(config.Template("x")))
		Expect(ok.Write).To(Equal(config.Template("x")))
		Expect(ok.Name).To(Equal("{{ .a }}"))
	})

	It("uses the inner YAML name for a Template field behind an inline embed", func() {
		o := templateOuterInline{Inner: templateInner{Read: `{{ .missing }}`}}

		err := config.RenderTemplates(&o, map[string]any{})

		Expect(err).To(HaveOccurred())
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("read"))
	})

	It("round trips a Template as a plain string in YAML and JSON", func() {
		h := templateHolder{Value: `host: {{ .a }}`}

		encoded, err := yaml.Marshal(h)
		Expect(err).NotTo(HaveOccurred())
		var decoded templateHolder
		err = yaml.Unmarshal(encoded, &decoded)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.Value).To(Equal(config.Template(`host: {{ .a }}`)))
		var asMap map[string]any
		err = yaml.Unmarshal(encoded, &asMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(asMap["value"]).To(Equal(`host: {{ .a }}`))

		encoded, err = json.Marshal(h)
		Expect(err).NotTo(HaveOccurred())
		err = json.Unmarshal(encoded, &decoded)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.Value).To(Equal(config.Template(`host: {{ .a }}`)))
		var asJSONMap map[string]any
		err = json.Unmarshal(encoded, &asJSONMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(asJSONMap["value"]).To(Equal(`host: {{ .a }}`))
	})

	It("guards print actions inside define, block and template bodies", func() {
		names := map[string]any{
			"m": map[string]any{"a": 1},
			"n": nil,
		}

		_, err := config.Template(`{{ define "x" }}{{ .m }}{{ end }}{{ template "x" . }}`).Render("read", names)
		Expect(err).To(HaveOccurred())

		_, err = config.Template(`{{ define "x" }}{{ .n }}{{ end }}{{ template "x" . }}`).Render("read", names)
		Expect(err).To(HaveOccurred())

		_, err = config.Template(`{{ block "b" . }}{{ .m }}{{ end }}`).Render("read", names)
		Expect(err).To(HaveOccurred())
	})

	It("guards print actions inside if, else and with bodies", func() {
		names := map[string]any{
			"t":   true,
			"obj": map[string]any{"m": map[string]any{"a": 1}},
		}

		_, err := config.Template(`{{ if .t }}{{ .obj.m }}{{ end }}`).Render("read", names)
		Expect(err).To(HaveOccurred())

		_, err = config.Template(`{{ if .t }}a{{ else }}{{ .obj.m }}{{ end }}`).Render("read", names)
		Expect(err).NotTo(HaveOccurred())

		_, err = config.Template(`{{ if .f }}a{{ else }}{{ .obj.m }}{{ end }}`).Render("read", names)
		Expect(err).To(HaveOccurred())

		_, err = config.Template(`{{ with .obj }}{{ .m }}{{ end }}`).Render("read", names)
		Expect(err).To(HaveOccurred())
	})

	It("errors when index prints a map value", func() {
		_, err := config.Template(`{{ index .mm "k" }}`).Render("read", map[string]any{
			"mm": map[string]any{"k": map[string]any{"a": 1}},
		})
		Expect(err).To(HaveOccurred())
	})

	It("errors when an action prints an array", func() {
		_, err := config.Template(`{{ .arr }}`).Render("read", map[string]any{
			"arr": [1]int{7},
		})
		Expect(err).To(HaveOccurred())
	})

	It("renders variable assignments without guarding them", func() {
		out, err := config.Template(`{{$x := .a}}{{ $x }}`).Render("read", map[string]any{"a": "x"})

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("x"))
	})

	It("fails on a missing key inside an if pipeline", func() {
		_, err := config.Template(`{{ if .missing }}x{{ end }}`).Render("read", map[string]any{})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing"))
	})

	It("leaves whole float64s outside int64 range in float form", func() {
		out, err := config.Template(`{{ .big }}`).Render("read", map[string]any{"big": float64(1e21)})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("1e+21"))

		out, err = config.Template(`{{ .inf }}`).Render("read", map[string]any{"inf": math.Inf(1)})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("+Inf"))
	})

	It("treats a missing key as a hard error, never ErrVariablesNotPropagated", func() {
		_, err := config.Template(`{{ .b }}`).Render("read", map[string]any{"a": "x"})

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(errors.Is(err, config.ErrVariablesNotPropagated)).To(BeFalse())
	})

	It("formats a ConfigError without a field as the wrapped error alone", func() {
		boom := errors.New("boom")
		Expect((&config.ConfigError{Err: boom}).Error()).To(Equal("boom"))
		Expect((&config.ConfigError{Field: "f", Err: boom}).Error()).To(Equal("f: boom"))
	})

	It("rejects non-struct pointers", func() {
		Expect(config.RenderTemplates("x", nil)).NotTo(BeNil())
		Expect(config.RenderTemplates(nil, nil)).NotTo(BeNil())
		Expect(config.RenderTemplates(&[]string{}, nil)).NotTo(BeNil())
	})

	It("renders a Template behind a pointer to a struct", func() {
		s := templatePtrOuter{Inner: &templateInner{Read: `{{ .a }}`}}
		err := config.RenderTemplates(&s, map[string]any{"a": "x"})
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Inner.Read).To(Equal(config.Template("x")))

		bad := templatePtrOuter{Inner: &templateInner{Read: `{{ .missing }}`}}
		err = config.RenderTemplates(&bad, map[string]any{})
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("inner.read"))
	})

	It("renders nothing behind a nil pointer", func() {
		s := templatePtrOuter{}

		err := config.RenderTemplates(&s, map[string]any{})

		Expect(err).NotTo(HaveOccurred())
		Expect(s.Inner).To(BeNil())
	})

	It("renders a Template inside a slice of structs", func() {
		s := templateSliceOuter{Items: []templateInner{{Read: `{{ .a }}`}}}
		err := config.RenderTemplates(&s, map[string]any{"a": "x"})
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Items[0].Read).To(Equal(config.Template("x")))

		bad := templateSliceOuter{Items: []templateInner{{Read: `{{ .missing }}`}}}
		err = config.RenderTemplates(&bad, map[string]any{})
		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("items[0].read"))
	})

	It("renders a Template field declared as a pointer", func() {
		tpl := config.Template(`{{ .a }}`)
		s := templatePtrField{Read: &tpl}
		err := config.RenderTemplates(&s, map[string]any{"a": "x"})
		Expect(err).NotTo(HaveOccurred())
		Expect(*s.Read).To(Equal(config.Template("x")))
	})

	It("errors instead of skipping a Template inside a map", func() {
		s := templateMapOuter{Items: map[string]templateInner{"k": {Read: `{{ .a }}`}}}

		err := config.RenderTemplates(&s, map[string]any{"a": "x"})

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("items.read"))
	})

	It("names a Template behind a nested struct by the joined YAML path", func() {
		bad := templateTaggedOuter{Inner: templateInner{Read: `{{ .missing }}`}}

		err := config.RenderTemplates(&bad, map[string]any{})

		var cfgErr *config.ConfigError
		Expect(errors.As(err, &cfgErr)).To(BeTrue())
		Expect(cfgErr.Field).To(Equal("inner.read"))
	})

	It("renders a Template tagged yaml:\"-\" under its Go name", func() {
		s := templateHidden{Value: `{{ .a }}`}

		err := config.RenderTemplates(&s, map[string]any{"a": "x"})

		Expect(err).NotTo(HaveOccurred())
		Expect(s.Value).To(Equal(config.Template("x")))
	})

	It("skips unexported fields silently", func() {
		s := templateUnexported{hidden: "x", Read: `{{ .a }}`}

		err := config.RenderTemplates(&s, map[string]any{"a": "x"})

		Expect(err).NotTo(HaveOccurred())
		Expect(s.Read).To(Equal(config.Template("x")))
	})
})
