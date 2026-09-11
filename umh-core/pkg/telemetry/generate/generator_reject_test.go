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
	"strings"
	"testing"
)

// Every case here would otherwise reach a commit as a generated file that does
// not compile, or as an entry whose severity nobody chose.
func TestGenerateRejects(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantKey string
	}{
		{"missing brief", "cpu:\n  read_failed:\n    severity: warning\n", "read_failed"},
		{"missing severity", "cpu:\n  read_failed:\n    brief: x\n", "read_failed"},
		{"unknown severity", "cpu:\n  a:\n    brief: x\n    severity: fatal\n", "fatal"},
		{"go path collision, leaf vs branch", "cpu:\n  push:\n    brief: x\n    severity: warning\n  push::failed:\n    brief: y\n    severity: warning\n", "push"},
		{"go path collision, underscores", "cpu:\n  a_b:\n    brief: x\n    severity: warning\n  a__b:\n    brief: y\n    severity: warning\n", "a__b"},
		{"leading separator", "cpu:\n  ::a:\n    brief: x\n    severity: warning\n", "::a"},
		// YAML reads "a:::" as the key "a::", the last colon being the mapping
		// separator, so the error names what the parser saw.
		{"trailing separator", "cpu:\n  a:::\n    brief: x\n    severity: warning\n", "a::"},
		{"illegal character", "cpu:\n  a-b:\n    brief: x\n    severity: warning\n", "a-b"},
		{"go keyword segment", "cpu:\n  range:\n    brief: x\n    severity: warning\n", "range"},
		{"leading digit", "cpu:\n  1st_try:\n    brief: x\n    severity: warning\n", "1st_try"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Generate([]byte(testCase.yaml), "telemetry")
			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), testCase.wantKey) {
				t.Errorf("error %q does not name %q", err, testCase.wantKey)
			}
		})
	}
}

// Duplicate keys are yaml.v3's check, not the generator's. Pinning it here
// records where the check lives, so nobody reimplements it.
func TestDuplicateKeysAreRejectedByTheParser(t *testing.T) {
	_, err := Generate([]byte("cpu:\n  a::b:\n    brief: x\n    severity: warning\n  a::b:\n    brief: y\n    severity: error\n"), "telemetry")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	if !strings.Contains(err.Error(), "a::b") || !strings.Contains(err.Error(), "already defined") {
		t.Errorf("error %q does not report a redefined key", err)
	}
}

func TestGenerateAcceptsEmptyDomain(t *testing.T) {
	// A domain with no entries is a work-in-progress edit, not an error.
	out, err := Generate([]byte("cpu:\n"), "telemetry")
	if err != nil {
		t.Fatalf("empty domain rejected: %v", err)
	}

	if strings.Contains(string(out), "var Cpu") {
		t.Error("emitted a var for a domain with no entries")
	}
}
