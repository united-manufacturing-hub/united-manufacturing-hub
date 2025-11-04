package templating_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/templating"
)

func TestTemplating(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FSMv2 Templating Suite")
}

var _ = Describe("RenderTemplate", func() {
	Describe("Valid template rendering", func() {
		It("should render template with simple variable", func() {
			tmpl := "Hello {{ .Name }}"
			data := struct{ Name string }{Name: "World"}

			result, err := templating.RenderTemplate(tmpl, data)

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

			result, err := templating.RenderTemplate(tmpl, data)

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

			result, err := templating.RenderTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("https://api.example.com/api"))
		})

		It("should render empty template as empty string", func() {
			tmpl := ""
			data := struct{ Name string }{Name: "World"}

			result, err := templating.RenderTemplate(tmpl, data)

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

			result, err := templating.RenderTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`path=/data/logs, quote="test", newline=line1
line2`))
		})
	})

	Describe("Strict mode enforcement", func() {
		It("should return error when variable is missing (strict mode)", func() {
			tmpl := "Hello {{ .TYPO }}"
			data := struct{ Name string }{Name: "World"}

			result, err := templating.RenderTemplate(tmpl, data)

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

			result, err := templating.RenderTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execute template"))
			Expect(result).To(Equal(""))
		})
	})

	Describe("Parse error handling", func() {
		It("should return error for invalid template syntax", func() {
			tmpl := "Hello {{ .Name }"
			data := struct{ Name string }{Name: "World"}

			result, err := templating.RenderTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse template"))
			Expect(result).To(Equal(""))
		})

		It("should return descriptive error for parse errors", func() {
			tmpl := "{{ .Name"
			data := struct{ Name string }{Name: "World"}

			_, err := templating.RenderTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse template"))
		})
	})
})
