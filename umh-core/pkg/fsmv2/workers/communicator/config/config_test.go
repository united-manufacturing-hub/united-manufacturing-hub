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
	"testing"
)

func TestDefaultCommunicatorConfig_UseFSMv2TransportDefaultsFalse(t *testing.T) {
	cfg := DefaultCommunicatorConfig()

	if cfg.UseFSMv2Transport != false {
		t.Errorf("expected UseFSMv2Transport to default to false, got %v", cfg.UseFSMv2Transport)
	}
}

func TestCommunicatorConfig_UseFSMv2TransportCanBeEnabled(t *testing.T) {
	cfg := CommunicatorConfig{
		UseFSMv2Transport: true,
		APIURL:            "https://example.com",
		AuthToken:         "test-token",
	}

	if cfg.UseFSMv2Transport != true {
		t.Errorf("expected UseFSMv2Transport to be true, got %v", cfg.UseFSMv2Transport)
	}
}

func TestCommunicatorConfig_ValidateRequiresAPIURLWhenFSMv2Enabled(t *testing.T) {
	cfg := CommunicatorConfig{
		UseFSMv2Transport: true,
		APIURL:            "",
		AuthToken:         "test-token",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error when APIURL is empty and UseFSMv2Transport is true")
	}
}

func TestCommunicatorConfig_ValidateRequiresAuthTokenWhenFSMv2Enabled(t *testing.T) {
	cfg := CommunicatorConfig{
		UseFSMv2Transport: true,
		APIURL:            "https://example.com",
		AuthToken:         "",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error when AuthToken is empty and UseFSMv2Transport is true")
	}
}

func TestCommunicatorConfig_ValidatePassesWithAllRequiredFields(t *testing.T) {
	cfg := CommunicatorConfig{
		UseFSMv2Transport: true,
		APIURL:            "https://example.com",
		AuthToken:         "test-token",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error with all required fields, got %v", err)
	}
}

func TestCommunicatorConfig_ValidatePassesWhenFSMv2Disabled(t *testing.T) {
	cfg := CommunicatorConfig{
		UseFSMv2Transport: false,
		APIURL:            "",
		AuthToken:         "",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error when UseFSMv2Transport is false, got %v", err)
	}
}
