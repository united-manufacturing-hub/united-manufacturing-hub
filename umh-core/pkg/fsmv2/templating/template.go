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


package templating

import (
	"bytes"
	"fmt"
	"text/template"
)

// RenderTemplate renders a Go template string with the provided data using strict mode.
// Strict mode (missingkey=error) causes template execution to fail if any variable
// referenced in the template is not present in the data, helping catch configuration
// mistakes early.
//
// The function is generic and accepts any data type T as input.
//
// Returns the rendered template string or an error if:
//   - Template parsing fails (invalid template syntax)
//   - Template execution fails (missing variables in strict mode, or other runtime errors)
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
