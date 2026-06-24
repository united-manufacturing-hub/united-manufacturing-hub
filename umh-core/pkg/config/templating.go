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
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"unicode/utf8"

	"errors"

	"gopkg.in/yaml.v3"
)

// hasAnchors reports whether the given YAML node ─ *or any of its
// descendants* ─ defines an anchor (`&name`) **or** is an alias
// (`*name`).
//
// In YAML terminology:
//
//   - **Anchor**  – a node that can later be referenced (`&base …`)
//   - **Alias**   – a node that *is* such a reference (`*base`)
//
// Why we need it
// --------------
// The UMH configuration file lets operators use YAML templating.
// We therefore want to detect whether a mapping still
// contains anchors/aliases and, if so, refuse automatic mutations
// (currently only Edit operations) so that we never flatten or overwrite a handcrafted
// template.
//
// Behaviour
// ---------
//
//	hasAnchors(nil)                     == false
//
//	hasAnchors("&tpl {key: 1}")         == true   // anchor
//	hasAnchors("*tpl")                  == true   // alias
//
//	hasAnchors("plain: {nested: 1}")    == false
//
//	// ─ recursively true because of the child ─
//	hasAnchors("root: {cfg: *tpl}")     == true
func hasAnchors(n *yaml.Node) bool {
	if n == nil {
		return false
	}

	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		return true
	}

	for _, c := range n.Content {
		if hasAnchors(c) {
			return true
		}
	}

	return false
}

// UnmarshalYAML customises decoding so that we remember whether this
// particular Data-Flow-Component *instance* still relies on YAML
// anchors/aliases inside its               ┌──────────────────────────┐
//
//	dataFlowComponentConfig              │    templatable subtree   │
//	                                     └──────────────────────────┘
//
// The standard yaml.v3 decoder expands aliases eagerly and discards
// anchor metadata.  That is fine for runtime use but we still need to
// know *if* templating was used, because:
//
//   - If **yes** → the API helpers (`AtomicEdit…`, `AtomicDelete…`)
//     must refuse to modify this DFC automatically.
//   - If **no**  → the helpers may proceed and rewrite the config.
//
// The top-level loader (`parseConfig`) doesn't need to be aware of any
// of this: it simply decodes into `FullConfig` and the magic happens
// inside every DFC.
func (d *DataFlowComponentConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain DataFlowComponentConfig // prevent recursion

	var tmp plain

	// 1. decode into the temporary value just like the default behaviour
	if err := value.Decode(&tmp); err != nil {
		return err
	}

	// 2. locate the child node with key "dataFlowComponentConfig"
	// Note: In YAML v3, mapping nodes store key-value pairs in Content as alternating elements:
	// Content[0]=key1, Content[1]=value1, Content[2]=key2, Content[3]=value2, etc.
	// We iterate by 2 to step through each key-value pair.
	var cfgNode *yaml.Node

	if len(value.Content)%2 != 0 {
		return fmt.Errorf("invalid YAML mapping node: Content length %d is not even", len(value.Content))
	}

	for i := 0; i < len(value.Content); i += 2 {
		k, v := value.Content[i], value.Content[i+1]
		if k.Value == "dataFlowComponentConfig" {
			cfgNode = v

			break
		}
	}

	// 3. copy decoded data into the receiver
	*d = DataFlowComponentConfig(tmp)

	// 4. flag = true when that subtree has &anchor or *alias
	d.hasAnchors = hasAnchors(cfgNode) // fn shown below

	return nil
}

// UnmarshalYAML is a helper function to detect anchors and set the hasAnchors flag
// See also UnmarshalYAML for DataFlowComponentConfig.
func (d *ProtocolConverterConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain ProtocolConverterConfig // prevent recursion

	var tmp plain

	// 1. decode into the temporary value just like the default behaviour
	if err := value.Decode(&tmp); err != nil {
		return err
	}

	// 2. locate the child node with key "protocolConverterConfig"
	// Note: In YAML v3, mapping nodes store key-value pairs in Content as alternating elements:
	// Content[0]=key1, Content[1]=value1, Content[2]=key2, Content[3]=value2, etc.
	// We iterate by 2 to step through each key-value pair.
	var cfgNode *yaml.Node

	if len(value.Content)%2 != 0 {
		return fmt.Errorf("invalid YAML mapping node: Content length %d is not even", len(value.Content))
	}

	for i := 0; i < len(value.Content); i += 2 {
		k, v := value.Content[i], value.Content[i+1]
		if k.Value == "protocolConverterConfig" {
			cfgNode = v

			break
		}
	}

	// 3. copy decoded data into the receiver
	*d = ProtocolConverterConfig(tmp)

	// 4. flag = true when that subtree has &anchor or *alias
	d.hasAnchors = hasAnchors(cfgNode)

	return nil
}

// UnmarshalYAML is a helper function to detect anchors and set the hasAnchors flag
// See also UnmarshalYAML for DataFlowComponentConfig and ProtocolConverterConfig.
func (d *StreamProcessorConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain StreamProcessorConfig // prevent recursion

	var tmp plain

	// 1. decode into the temporary value just like the default behaviour
	if err := value.Decode(&tmp); err != nil {
		return err
	}

	// 2. locate the child node with key "streamProcessorServiceConfig"
	// Note: In YAML v3, mapping nodes store key-value pairs in Content as alternating elements:
	// Content[0]=key1, Content[1]=value1, Content[2]=key2, Content[3]=value2, etc.
	// We iterate by 2 to step through each key-value pair.
	var cfgNode *yaml.Node

	if len(value.Content)%2 != 0 {
		return fmt.Errorf("invalid YAML mapping node: Content length %d is not even", len(value.Content))
	}

	for i := 0; i < len(value.Content); i += 2 {
		k, v := value.Content[i], value.Content[i+1]
		if k.Value == "streamProcessorServiceConfig" {
			cfgNode = v

			break
		}
	}

	// 3. copy decoded data into the receiver
	*d = StreamProcessorConfig(tmp)

	// 4. flag = true when that subtree has &anchor or *alias
	d.hasAnchors = hasAnchors(cfgNode)

	return nil
}

// TemplateRenderError is returned by RenderTemplate when YAML unmarshalling
// of the rendered output fails. It carries the raw YAML error and the
// rendered-region snippet separately so callers that compose user-facing
// messages can present clean, un-prefixed detail without repackaging the
// error string. Error() still returns the same string as the plain
// fmt.Errorf wrapper it replaces, so all other callers are unaffected.
type TemplateRenderError struct {
	// YAMLErr is the raw yaml.Unmarshal error, e.g. "yaml: line 10: …".
	YAMLErr error
	// Snippet is the rendered-region context block (starts with "\n").
	Snippet string
}

func (e *TemplateRenderError) Error() string {
	return "failed to render template as valid YAML: " + e.YAMLErr.Error() + e.Snippet
}

func (e *TemplateRenderError) Unwrap() error { return e.YAMLErr }

// RenderTemplate takes an *arbitrary* struct that still contains
// {{ … }} actions, renders it with text/template and returns the same
// struct type fully materialised.
//
// Callers **must** supply a fully-merged variable scope; the function does
// not fetch or inject `.global`, `.internal`, or `.location` keys.
func RenderTemplate[T any](tmpl T, scope map[string]any) (T, error) {
	if scope == nil {
		return *new(T), errors.New("scope cannot be nil")
	}

	// A. serialise to YAML – keeps anchors & order stable for diffing
	raw, err := yaml.Marshal(tmpl)
	if err != nil {
		return *new(T), fmt.Errorf("failed to marshal template to YAML: %w", err)
	}

	// B. parse + execute the template (no extra FuncMap – sandboxed!)
	tpl, err := template.New("pc").Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return *new(T), fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, scope); err != nil {
		return *new(T), fmt.Errorf("failed to execute template: %w", err)
	}

	// C. unmarshal back into the *same* Go type
	var out T
	if err := yaml.Unmarshal(buf.Bytes(), &out); err != nil {
		return *new(T), &TemplateRenderError{
			YAMLErr: err,
			Snippet: renderedRegionSnippet(buf.Bytes(), err),
		}
	}

	// D. sanity-check – no {{ left over
	if bytes.Contains(buf.Bytes(), []byte("{{")) {
		return *new(T), errors.New("unresolved template markers in rendered output")
	}

	return out, nil
}

// yamlErrorLineRegex extracts the line number from yaml.v3 error messages,
// which come as `yaml: line N: <reason>` (parser and scanner errors) or
// `line N: <reason>` items inside a yaml.TypeError.
var yamlErrorLineRegex = regexp.MustCompile(`\bline (\d+):`)

// maxSnippetRegions caps how many error regions a single snippet shows when a
// yaml.TypeError reports many failing lines.
const maxSnippetRegions = 3

// maxSnippetLineLength caps each emitted snippet line so a long single-line
// value cannot balloon the error string (which is re-logged every reconcile
// tick and pushed to the Management Console via the status reason).
const maxSnippetLineLength = 160

// renderedRegionSnippet returns the regions of the rendered output around the
// lines a yaml unmarshal error points at, formatted with 1-based line numbers
// and a `>` marker on each reported line. The marker is approximate: yaml.v3
// parser errors report 0-based lines while scanner and type errors report
// 1-based ones, so the malformed content can sit one line below the marker.
// Reported lines outside the rendered output clamp to the nearest line
// (parser errors on the first line report "line 0"). When the error carries
// no line number at all, the head of the rendered output is shown instead.
// Without this the rendered output is discarded on failure and the error is
// undiagnosable from logs (ENG-5103).
func renderedRegionSnippet(rendered []byte, yamlErr error) string {
	var errLines []int

	var typeErr *yaml.TypeError
	if errors.As(yamlErr, &typeErr) {
		seen := make(map[int]bool, len(typeErr.Errors))

		for _, e := range typeErr.Errors {
			match := yamlErrorLineRegex.FindStringSubmatch(e)
			if match == nil {
				continue
			}

			n, err := strconv.Atoi(match[1])
			if err != nil || seen[n] {
				continue
			}

			seen[n] = true
			errLines = append(errLines, n)
		}
	} else if match := yamlErrorLineRegex.FindStringSubmatch(yamlErr.Error()); match != nil {
		n, err := strconv.Atoi(match[1])
		if err == nil {
			errLines = append(errLines, n)
		}
	}

	if len(errLines) > maxSnippetRegions {
		errLines = errLines[:maxSnippetRegions]
	}

	// Rendered output ends with one trailing newline, or two when the last
	// value is a keep-chomped (|+) block scalar; without trimming them the
	// split leaves phantom empty last lines that out-of-range errors clamp
	// to.
	lines := strings.Split(strings.TrimRight(string(rendered), "\n"), "\n")

	const contextLines = 3

	var b strings.Builder

	if len(errLines) == 0 {
		// Some yaml.v3 errors carry no position (e.g. "mapping values are
		// not allowed in this context", unknown anchors). Show the head of
		// the rendered output rather than discarding it entirely.
		b.WriteString("\nrendered output (yaml error reported no line number), first lines:\n")

		end := min(len(lines), 1+2*contextLines)
		for i := 1; i <= end; i++ {
			fmt.Fprintf(&b, "  %4d | %s\n", i, truncateSnippetLine(lines[i-1]))
		}

		return strings.TrimSuffix(b.String(), "\n")
	}

	for _, errLine := range errLines {
		errLine = min(max(errLine, 1), len(lines))

		if b.Len() == 0 {
			b.WriteString("\nrendered output around the reported line (yaml line numbers can be off by one):\n")
		} else {
			b.WriteString("...\n")
		}

		start := max(1, errLine-contextLines)
		end := min(len(lines), errLine+contextLines)

		for i := start; i <= end; i++ {
			marker := "  "
			if i == errLine {
				marker = "> "
			}

			fmt.Fprintf(&b, "%s%4d | %s\n", marker, i, truncateSnippetLine(lines[i-1]))
		}
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// truncateSnippetLine shortens a single snippet line to maxSnippetLineLength,
// cutting on a rune boundary so the result stays valid UTF-8.
func truncateSnippetLine(line string) string {
	if len(line) <= maxSnippetLineLength {
		return line
	}

	cut := maxSnippetLineLength
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}

	return line[:cut] + "…(truncated)"
}
