package templating

import (
	"bytes"
	"fmt"
	"text/template"
)

func RenderTemplate[T any](tmpl string, data T) (string, error) {
	t := template.New("config").Option("missingkey=error")

	t, err := t.Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer

	err = t.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
