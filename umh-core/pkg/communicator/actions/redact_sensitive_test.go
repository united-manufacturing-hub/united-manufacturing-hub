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

package actions

import (
	"fmt"
	"strings"
	"testing"
)

func TestRedactSensitiveMasksSecretKeys(t *testing.T) {
	payload := map[string]interface{}{
		"host":     "timescale.example.com",
		"password": "super-secret",
		"port":     float64(5432),
		"apiKey":   "abc123",
		"nested": map[string]interface{}{
			"token": "tok-999",
			"user":  "umh_owner",
		},
		"list": []interface{}{
			map[string]interface{}{"credential": "c-1"},
		},
	}

	got := fmt.Sprintf("%v", redactSensitive(payload))

	for _, secret := range []string{"super-secret", "abc123", "tok-999", "c-1"} {
		if strings.Contains(got, secret) {
			t.Errorf("redacted output still contains secret %q: %s", secret, got)
		}
	}
	for _, kept := range []string{"timescale.example.com", "umh_owner", "5432"} {
		if !strings.Contains(got, kept) {
			t.Errorf("redacted output dropped non-secret %q: %s", kept, got)
		}
	}
}

func TestRedactSensitiveLeavesOriginalUntouched(t *testing.T) {
	payload := map[string]interface{}{"password": "super-secret"}

	redactSensitive(payload)

	if payload["password"] != "super-secret" {
		t.Errorf("redactSensitive mutated the original payload: %v", payload)
	}
}

func TestRedactSensitivePassesThroughNonMaps(t *testing.T) {
	if got := redactSensitive("plain"); got != "plain" {
		t.Errorf("expected passthrough for scalar, got %v", got)
	}
	if got := redactSensitive(nil); got != nil {
		t.Errorf("expected nil passthrough, got %v", got)
	}
}
