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
	"text/template"

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
// (Add/Edit/Delete) so that we never flatten or overwrite a handcrafted
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
// The top-level loader (`parseConfig`) doesn’t need to be aware of any
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
	var cfgNode *yaml.Node
	for i := 0; i < len(value.Content)-1; i += 2 {
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
// See also UnmarshalYAML for DataFlowComponentConfig
func (d *ProtocolConverterConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain ProtocolConverterConfig // prevent recursion
	var tmp plain

	// 1. decode into the temporary value just like the default behaviour
	if err := value.Decode(&tmp); err != nil {
		return err
	}

	// 2. locate the child node with key "protocolConverterConfig"
	var cfgNode *yaml.Node
	for i := 0; i < len(value.Content)-1; i += 2 {
		k, v := value.Content[i], value.Content[i+1]
		if k.Value == "protocolConverterConfig" {
			cfgNode = v
			break
		}
	}

	// 3. copy decoded data into the receiver
	*d = ProtocolConverterConfig(tmp)

	// 4. flag = true when that subtree has &anchor or *alias
	d.hasAnchors = hasAnchors(cfgNode) // fn shown below

	return nil
}

// RenderTemplate takes an *arbitrary* struct that still contains
// {{ … }} actions, renders it with text/template and returns the same
// struct type fully materialised.
//
// Callers **must** supply a fully-merged variable scope; the function does
// not fetch or inject `.global`, `.internal`, or `.location` keys.
func RenderTemplate[T any](tmpl T, scope map[string]any) (T, error) {
	// A. serialise to YAML – keeps anchors & order stable for diffing
	raw, err := yaml.Marshal(tmpl)
	if err != nil {
		return *new(T), err
	}

	// B. parse + execute the template (no extra FuncMap – sandboxed!)
	tpl, err := template.New("pc").Parse(string(raw))
	if err != nil {
		return *new(T), err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, scope); err != nil {
		return *new(T), err
	}

	// C. unmarshal back into the *same* Go type
	var out T
	if err := yaml.Unmarshal(buf.Bytes(), &out); err != nil {
		return *new(T), err
	}

	// D. sanity-check – no {{ left over
	if bytes.Contains(buf.Bytes(), []byte("{{")) {
		return *new(T), fmt.Errorf("unresolved template markers in %T", tmpl)
	}
	return out, nil
}
