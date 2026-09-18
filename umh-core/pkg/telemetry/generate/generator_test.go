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
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const twoEntryYAML = `
cpu:
  read_failed:
    brief: A cgroup CPU file could not be read; the measurement continues.
    severity: warning
transport:
  push::persistent_failure:
    brief: Outbound pushes keep failing; the instance may look offline.
    severity: error
`

func TestGenerateEmitsOneVarPerDomain(t *testing.T) {
	out, err := Generate([]byte(twoEntryYAML), "telemetry")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out)
	for _, want := range []string{
		"// Code generated from telemetry.yaml. DO NOT EDIT.",
		"Copyright 2025 UMH Systems GmbH",
		"package telemetry",
		`var Cpu = cpuNode{`,
		`var Transport = transportNode{`,
		`Tag: "cpu::read_failed"`,
		`Tag: "transport::push::persistent_failure"`,
		`Severity: SeverityWarning`,
		`Severity: SeverityError`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestGenerateNestsOnSeparator(t *testing.T) {
	out, _ := Generate([]byte(twoEntryYAML), "telemetry")

	got := string(out)
	// push::persistent_failure is two segments, so Push is a struct holding
	// PersistentFailure. Flattening it to PushPersistentFailure would make the
	// call site telemetry.Transport.PushPersistentFailure, and a later entry
	// under push would then have nowhere to hang.
	if !strings.Contains(got, "Push transportPushNode") {
		t.Error("expected a nested node type for the push segment")
	}

	if strings.Contains(got, "PushPersistentFailure") {
		t.Error("segments were flattened instead of nested")
	}
}

func TestGenerateIsSortedAndDeterministic(t *testing.T) {
	first, _ := Generate([]byte(twoEntryYAML), "telemetry")
	second, _ := Generate([]byte(twoEntryYAML), "telemetry")

	// Rung 5 compares the committed file byte for byte against a fresh run, so a
	// map iteration anywhere in the emit path fails there intermittently rather
	// than here.
	if string(first) != string(second) {
		t.Fatal("two runs over identical input differ")
	}

	got := string(first)
	if strings.Index(got, "var Cpu") > strings.Index(got, "var Transport") {
		t.Error("domains are not emitted in sorted order")
	}
}

func TestGeneratedSourceParses(t *testing.T) {
	out, err := Generate([]byte(twoEntryYAML), "telemetry")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// The generator writes Go, and nothing downstream compiles its output until
	// rung 5. Parsing it here is what stops a malformed emission reaching a
	// commit.
	if _, err := parser.ParseFile(token.NewFileSet(), "identifiers.gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, out)
	}
}
