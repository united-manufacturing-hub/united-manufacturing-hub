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

package env

import (
	"strings"
	"testing"
)

// TestGetAsBoolStrictReportsUnrecognisedValues pins the one way
// GetAsBoolStrict differs from GetAsBool: a value that is set but is not one of
// the recognised spellings comes back with an error instead of being folded
// into defaultValue. Unset stays error-free, because callers use an unset
// variable as their normal default.
func TestGetAsBoolStrictReportsUnrecognisedValues(t *testing.T) {
	const key = "UMH_TEST_GET_AS_BOOL_STRICT"

	tests := []struct {
		name       string
		set        bool
		value      string
		wantParsed bool
		wantErr    bool
	}{
		{name: "unset is not an error", set: false, wantParsed: false},
		{name: "recognised true", set: true, value: "true", wantParsed: true},
		{name: "recognised on", set: true, value: "on", wantParsed: true},
		{name: "recognised ON", set: true, value: "ON", wantParsed: true},
		{name: "recognised false", set: true, value: "false", wantParsed: false},
		{name: "recognised off", set: true, value: "off", wantParsed: false},
		{name: "unrecognised maybe", set: true, value: "maybe", wantParsed: false, wantErr: true},
		{name: "unrecognised typo of true", set: true, value: "ture", wantParsed: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(key, tt.value)
			} else {
				t.Setenv(key, "")
			}

			parsed, err := GetAsBoolStrict(key, false)

			if parsed != tt.wantParsed {
				t.Errorf("GetAsBoolStrict(%q=%q) parsed = %v, want %v", key, tt.value, parsed, tt.wantParsed)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetAsBoolStrict(%q=%q) returned no error; an unrecognised value must be reported, not swallowed", key, tt.value)
				}

				// The caller logs this error verbatim, so the message has to
				// carry both the variable name and the offending value.
				if !strings.Contains(err.Error(), key) {
					t.Errorf("error %q does not name the variable %q", err, key)
				}

				if !strings.Contains(err.Error(), tt.value) {
					t.Errorf("error %q does not quote the offending value %q", err, tt.value)
				}

				return
			}

			if err != nil {
				t.Errorf("GetAsBoolStrict(%q=%q) returned unexpected error: %v", key, tt.value, err)
			}
		})
	}
}

// TestGetAsBoolStrictHonoursDefaultOnUnrecognisedValue pins that the parsed
// value returned alongside the error is defaultValue, so a caller that only
// logs the error still behaves exactly like GetAsBool.
func TestGetAsBoolStrictHonoursDefaultOnUnrecognisedValue(t *testing.T) {
	const key = "UMH_TEST_GET_AS_BOOL_STRICT_DEFAULT"

	t.Setenv(key, "maybe")

	parsed, err := GetAsBoolStrict(key, true)
	if err == nil {
		t.Fatal("expected an error for the unrecognised value maybe")
	}

	if !parsed {
		t.Error("parsed value alongside the error must be defaultValue (true), so an error-ignoring caller matches GetAsBool")
	}
}
