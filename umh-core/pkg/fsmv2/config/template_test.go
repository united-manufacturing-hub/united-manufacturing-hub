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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("RenderTemplate", func() {
	Describe("Valid template rendering", func() {
		It("should render template with simple variable", func() {
			tmpl := "Hello {{ .Name }}"
			data := struct{ Name string }{Name: "World"}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Hello World"))
		})

		It("should render template with multiple variables", func() {
			tmpl := "{{ .Protocol }}://{{ .IP }}:{{ .Port }}"
			data := struct {
				Protocol string
				IP       string
				Port     int
			}{
				Protocol: "mqtt",
				IP:       "192.168.1.100",
				Port:     1883,
			}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("mqtt://192.168.1.100:1883"))
		})

		It("should render template with nested data", func() {
			tmpl := "{{ .Global.ApiEndpoint }}/api"
			data := struct {
				Global struct {
					ApiEndpoint string
				}
			}{
				Global: struct {
					ApiEndpoint string
				}{
					ApiEndpoint: "https://api.example.com",
				},
			}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("https://api.example.com/api"))
		})

		It("should render empty template as empty string", func() {
			tmpl := ""
			data := struct{ Name string }{Name: "World"}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(""))
		})

		It("should render template with special characters", func() {
			tmpl := `path={{ .Path }}, quote="{{ .Quote }}", newline={{ .Newline }}`
			data := struct {
				Path    string
				Quote   string
				Newline string
			}{
				Path:    "/data/logs",
				Quote:   "test",
				Newline: "line1\nline2",
			}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`path=/data/logs, quote="test", newline=line1
line2`))
		})
	})

	Describe("Strict mode enforcement", func() {
		It("should return error when variable is missing (strict mode)", func() {
			tmpl := "Hello {{ .TYPO }}"
			data := struct{ Name string }{Name: "World"}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execute template"))
			Expect(err.Error()).To(Or(
				ContainSubstring("TYPO"),
				ContainSubstring("can't evaluate field TYPO"),
			))
			Expect(result).To(Equal(""))
		})

		It("should return error when accessing nested field that doesn't exist", func() {
			tmpl := "{{ .Config.Missing }}"
			data := struct {
				Config struct {
					Existing string
				}
			}{
				Config: struct{ Existing string }{
					Existing: "value",
				},
			}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execute template"))
			Expect(result).To(Equal(""))
		})
	})

	Describe("Parse error handling", func() {
		It("should return error for invalid template syntax", func() {
			tmpl := "Hello {{ .Name }"
			data := struct{ Name string }{Name: "World"}

			result, err := config.RenderTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse template"))
			Expect(result).To(Equal(""))
		})

		It("should return descriptive error for parse errors", func() {
			tmpl := "{{ .Name"
			data := struct{ Name string }{Name: "World"}

			_, err := config.RenderTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse template"))
		})
	})

	Describe("Performance", func() {
		It("should render 100 templates in <500ms (<5ms per template)", func() {
			tmpl := "{{ .Protocol }}://{{ .IP }}:{{ .Port }}/{{ .Path }}"
			templateData := make([]struct {
				Protocol string
				IP       string
				Port     int
				Path     string
			}, 100)

			for i := range 100 {
				templateData[i] = struct {
					Protocol string
					IP       string
					Port     int
					Path     string
				}{
					Protocol: "mqtt",
					IP:       "192.168.1.100",
					Port:     1883,
					Path:     "data/sensors",
				}
			}

			start := time.Now()

			for i := range 100 {
				result, err := config.RenderTemplate(tmpl, templateData[i])
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("mqtt://192.168.1.100:1883/data/sensors"))
			}

			elapsed := time.Since(start)

			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond))
		})

		It("should render 100 complex templates with nested data in <500ms", func() {
			tmpl := `{
  "connection": {
    "protocol": "{{ .Protocol }}",
    "endpoint": "{{ .Endpoint.Host }}:{{ .Endpoint.Port }}",
    "path": "{{ .Path }}",
    "auth": {
      "type": "{{ .Auth.Type }}",
      "token": "{{ .Auth.Token }}"
    }
  }
}`

			type Auth struct {
				Type  string
				Token string
			}

			type Endpoint struct {
				Host string
				Port int
			}

			type ComplexData struct {
				Protocol string
				Endpoint Endpoint
				Path     string
				Auth     Auth
			}

			templateData := make([]ComplexData, 100)
			for i := range 100 {
				templateData[i] = ComplexData{
					Protocol: "https",
					Endpoint: Endpoint{Host: "api.example.com", Port: 443},
					Path:     "/v1/data",
					Auth:     Auth{Type: "bearer", Token: "secret-token-123"},
				}
			}

			start := time.Now()

			for i := range 100 {
				result, err := config.RenderTemplate(tmpl, templateData[i])
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring(`"protocol": "https"`))
				Expect(result).To(ContainSubstring(`"endpoint": "api.example.com:443"`))
			}

			elapsed := time.Since(start)

			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond))
		})
	})
})
