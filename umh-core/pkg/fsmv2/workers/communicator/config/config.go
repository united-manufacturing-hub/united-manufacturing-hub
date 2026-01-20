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
	"errors"
)

// Configuration errors.
var (
	ErrAPIURLRequired    = errors.New("APIURL is required when UseFSMv2Transport is enabled")
	ErrAuthTokenRequired = errors.New("AuthToken is required when UseFSMv2Transport is enabled")
)

// TransportResetThreshold is the number of consecutive errors before the
// transport is reset in degraded state. Reset occurs at multiples of this
// threshold (5, 10, 15...) to allow periodic reset attempts.
const TransportResetThreshold = 5

// CommunicatorConfig holds configuration for the communicator worker.
type CommunicatorConfig struct {
	// APIURL is the Management Console backend URL
	APIURL string `json:"apiURL" yaml:"apiURL"`

	// AuthToken is the pre-shared authentication token
	AuthToken string `json:"authToken" yaml:"authToken"`

	// UseFSMv2Transport enables the new FSMv2-based HTTP transport layer
	// Default: false (for backward compatibility)
	UseFSMv2Transport bool `json:"useFSMv2Transport" yaml:"useFSMv2Transport"`

	// AllowInsecureTLS skips TLS certificate verification (for testing only)
	AllowInsecureTLS bool `json:"allowInsecureTLS" yaml:"allowInsecureTLS"`
}

// DefaultCommunicatorConfig returns the default configuration.
func DefaultCommunicatorConfig() CommunicatorConfig {
	return CommunicatorConfig{
		UseFSMv2Transport: false,
	}
}

// Validate checks that the configuration is valid.
func (c *CommunicatorConfig) Validate() error {
	if !c.UseFSMv2Transport {
		return nil
	}

	if c.APIURL == "" {
		return ErrAPIURLRequired
	}

	if c.AuthToken == "" {
		return ErrAuthTokenRequired
	}

	return nil
}
