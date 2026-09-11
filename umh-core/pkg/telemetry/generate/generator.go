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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type declaredEvent struct {
	Brief    string `yaml:"brief"`
	Severity string `yaml:"severity"`
}

// node is one level of the emitted tree. A node with children is a branch and
// becomes a struct type; a node with an event is a leaf and becomes an
// Identifier field.
type node struct {
	children map[string]*node
	event    *declaredEvent
	tag      string
}

const licenseHeader = `// Copyright 2025 UMH Systems GmbH
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
`

// Generate turns telemetry.yaml into the Go source of identifiers.gen.go. Every
// level is emitted in sorted key order, because a YAML mapping decodes into an
// unordered Go map and rung 5 compares the committed file byte for byte.
func Generate(registryYAML []byte, packageName string) ([]byte, error) {
	domains := map[string]map[string]declaredEvent{}
	if err := yaml.Unmarshal(registryYAML, &domains); err != nil {
		return nil, fmt.Errorf("parse telemetry.yaml: %w", err)
	}

	roots := buildTree(domains)

	var source bytes.Buffer
	source.WriteString(licenseHeader)
	source.WriteString("\n// Code generated from telemetry.yaml. DO NOT EDIT.\n\n")
	fmt.Fprintf(&source, "package %s\n", packageName)

	for _, domain := range sortedKeys(roots) {
		typeName := goIdentifier(domain)
		emitNodeTypes(&source, typeName, roots[domain])
		emitVar(&source, domain, typeName, roots[domain])
	}

	formatted, err := format.Source(source.Bytes())
	if err != nil {
		return nil, fmt.Errorf("generated source does not parse: %w", err)
	}

	return formatted, nil
}

// buildTree splits each event key on "::" so the emitted tree follows the tag
// rather than the YAML's flat keys. A domain with no events yields no node, so
// nothing is emitted for it.
func buildTree(domains map[string]map[string]declaredEvent) map[string]*node {
	roots := map[string]*node{}

	for domain, events := range domains {
		if len(events) == 0 {
			continue
		}

		root := &node{children: map[string]*node{}}

		for key, event := range events {
			cursor := root
			for _, segment := range strings.Split(key, "::") {
				child, found := cursor.children[segment]
				if !found {
					child = &node{children: map[string]*node{}}
					cursor.children[segment] = child
				}

				cursor = child
			}

			leaf := event
			cursor.event = &leaf
			cursor.tag = domain + "::" + key
		}

		roots[domain] = root
	}

	return roots
}

// emitNodeTypes declares a struct type per branch, depth first, so a type is
// declared before the type that embeds it.
func emitNodeTypes(source *bytes.Buffer, typeName string, branch *node) {
	for _, segment := range sortedKeys(branch.children) {
		child := branch.children[segment]
		if len(child.children) > 0 {
			emitNodeTypes(source, typeName+goIdentifier(segment), child)
		}
	}

	fmt.Fprintf(source, "\ntype %sNode struct {\n", lowerFirst(typeName))

	for _, segment := range sortedKeys(branch.children) {
		child := branch.children[segment]
		if len(child.children) > 0 {
			fmt.Fprintf(source, "%s %sNode\n", goIdentifier(segment), lowerFirst(typeName+goIdentifier(segment)))

			continue
		}

		fmt.Fprintf(source, "%s Identifier\n", goIdentifier(segment))
	}

	source.WriteString("}\n")
}

func emitVar(source *bytes.Buffer, domain, typeName string, branch *node) {
	fmt.Fprintf(source, "\n// %s holds the declared events of the %s domain.\n", goIdentifier(domain), domain)
	fmt.Fprintf(source, "var %s = %sNode{\n", goIdentifier(domain), lowerFirst(typeName))
	emitFields(source, typeName, branch)
	source.WriteString("}\n")
}

func emitFields(source *bytes.Buffer, typeName string, branch *node) {
	for _, segment := range sortedKeys(branch.children) {
		child := branch.children[segment]
		if len(child.children) > 0 {
			fmt.Fprintf(source, "%s: %sNode{\n", goIdentifier(segment), lowerFirst(typeName+goIdentifier(segment)))
			emitFields(source, typeName+goIdentifier(segment), child)
			source.WriteString("},\n")

			continue
		}

		fmt.Fprintf(source, "%s: Identifier{Tag: %q, Brief: %q, Severity: Severity%s},\n",
			goIdentifier(segment), child.tag, child.event.Brief, goIdentifier(child.event.Severity))
	}
}

func sortedKeys[Value any](byKey map[string]Value) []string {
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// goIdentifier renders one segment as a Go identifier: "persistent_failure"
// becomes "PersistentFailure". Initialisms are deliberately not special-cased,
// so "cpu" becomes "Cpu" and the spec's naming rule stays the only rule.
func goIdentifier(segment string) string {
	var identifier strings.Builder

	for _, word := range strings.Split(segment, "_") {
		if word == "" {
			continue
		}

		identifier.WriteString(strings.ToUpper(word[:1]) + word[1:])
	}

	return identifier.String()
}

func lowerFirst(name string) string {
	if name == "" {
		return name
	}

	return strings.ToLower(name[:1]) + name[1:]
}

func main() {
	in := flag.String("in", "telemetry.yaml", "path to the source YAML")
	out := flag.String("out", "identifiers.gen.go", "path to the generated Go file")
	packageName := flag.String("package", "telemetry", "package name to emit")
	flag.Parse()

	registryYAML, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	generated, err := Generate(registryYAML, *packageName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Written only after Generate succeeds, so a malformed entry leaves the
	// previous file intact rather than truncating it.
	if err := os.WriteFile(*out, generated, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
