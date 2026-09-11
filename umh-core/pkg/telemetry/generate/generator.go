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
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// tagFormat is the spec's tag shape. Matching it is what rejects a leading,
// trailing or doubled separator and any character outside the set, so those
// three do not need checks of their own.
var tagFormat = regexp.MustCompile(`^[a-z0-9_]+(::[a-z0-9_]+)+$`)

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

	roots, err := buildTree(domains)
	if err != nil {
		return nil, err
	}

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
//
// Duplicate keys within one domain are not checked here: yaml.v3 rejects them
// during Unmarshal, naming the key and both line numbers.
func buildTree(domains map[string]map[string]declaredEvent) (map[string]*node, error) {
	roots := map[string]*node{}
	// Go path to the YAML key that claimed it. Two keys can differ and still
	// need the same Go field: "push" beside "push::failed" wants Push as both a
	// value and a struct, and "a__b" title-cases onto the same name as "a_b".
	claimedBy := map[string]string{}

	for _, domain := range sortedKeys(domains) {
		events := domains[domain]
		if len(events) == 0 {
			continue
		}

		root := &node{children: map[string]*node{}}

		for _, key := range sortedKeys(events) {
			event := events[key]

			if err := validate(domain, key, event); err != nil {
				return nil, err
			}

			cursor := root
			goPath := goIdentifier(domain)

			for _, segment := range strings.Split(key, "::") {
				goPath += "." + goIdentifier(segment)

				child, found := cursor.children[segment]
				if !found {
					child = &node{children: map[string]*node{}}
					cursor.children[segment] = child
				}

				cursor = child
			}

			if owner, taken := claimedBy[goPath]; taken {
				return nil, fmt.Errorf("keys %q and %q both need the Go path %s", owner, key, goPath)
			}

			claimedBy[goPath] = key

			leaf := event
			cursor.event = &leaf
			cursor.tag = domain + "::" + key
		}

		roots[domain] = root
	}

	// A leaf that also has children claimed its own path as a value and as a
	// struct, which cannot compile.
	for _, domain := range sortedKeys(roots) {
		if err := rejectLeafBranch(domain, goIdentifier(domain), roots[domain]); err != nil {
			return nil, err
		}
	}

	return roots, nil
}

func rejectLeafBranch(key, goPath string, branch *node) error {
	if branch.event != nil && len(branch.children) > 0 {
		return fmt.Errorf("key %q is both an event and a prefix of other events, so %s cannot be a value and a struct", key, goPath)
	}

	for _, segment := range sortedKeys(branch.children) {
		child := branch.children[segment]
		if err := rejectLeafBranch(segment, goPath+"."+goIdentifier(segment), child); err != nil {
			return err
		}
	}

	return nil
}

// validate rejects an entry the generator cannot turn into compiling Go, or one
// whose severity nobody chose. Every message names the offending key, because
// the YAML is hand-edited and the key is how the author finds it.
func validate(domain, key string, event declaredEvent) error {
	if event.Brief == "" {
		return fmt.Errorf("key %q has no brief", key)
	}

	if event.Severity == "" {
		return fmt.Errorf("key %q has no severity", key)
	}

	// No default: a defaulted severity is a silent decision about whether
	// somebody gets paged.
	if event.Severity != "warning" && event.Severity != "error" {
		return fmt.Errorf("key %q has severity %q, which is neither warning nor error", key, event.Severity)
	}

	tag := domain + "::" + key
	if !tagFormat.MatchString(tag) {
		return fmt.Errorf("key %q gives tag %q, which does not match %s", key, tag, tagFormat)
	}

	for _, segment := range strings.Split(key, "::") {
		name := goIdentifier(segment)
		if name == "" {
			return fmt.Errorf("key %q has an empty segment", key)
		}

		if token.IsKeyword(strings.ToLower(name)) {
			return fmt.Errorf("key %q has segment %q, a Go keyword", key, segment)
		}

		if name[0] >= '0' && name[0] <= '9' {
			return fmt.Errorf("key %q has segment %q, which starts with a digit", key, segment)
		}
	}

	return nil
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
