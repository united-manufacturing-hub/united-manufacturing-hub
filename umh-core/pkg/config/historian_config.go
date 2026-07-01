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
	"fmt"
)

// HistorianSSLMode controls TLS behaviour for the historian Postgres connection.
type HistorianSSLMode string

const (
	HistorianSSLModeRequire    HistorianSSLMode = "require"
	HistorianSSLModeDisable    HistorianSSLMode = "disable"
	HistorianSSLModeVerifyFull HistorianSSLMode = "verify-full"
)

// IsValid reports whether s is one of the three allowed SSL modes.
func (s HistorianSSLMode) IsValid() bool {
	switch s {
	case HistorianSSLModeRequire, HistorianSSLModeDisable, HistorianSSLModeVerifyFull:
		return true
	default:
		return false
	}
}

// HistorianConfig holds the connection settings for the TimescaleDB/Postgres historian.
// It maps to the top-level `historian:` key in config.yaml.
type HistorianConfig struct {
	// SSLRootCert is the path inside the container to the CA certificate file.
	SSLRootCert string `yaml:"sslrootcert,omitempty" json:"sslrootcert,omitempty"`
	// SSLCert is the path inside the container to the client certificate file.
	SSLCert string `yaml:"sslcert,omitempty" json:"sslcert,omitempty"`
	// SSLKey is the path inside the container to the client key file.
	SSLKey string `yaml:"sslkey,omitempty" json:"sslkey,omitempty"`
	// Host is the TimescaleDB/Postgres hostname or IP address (required).
	Host string `yaml:"host" json:"host"`
	// Password is the login role password (required). Redacted in logs.
	Password string `yaml:"password" json:"password"`
	// Database is the database name. Defaults to "umh".
	Database string `yaml:"database,omitempty" json:"database,omitempty"`
	// Username is the login role. Defaults to "umh_owner".
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	// SSLMode controls TLS behaviour. One of HistorianSSLModeRequire (default), HistorianSSLModeDisable, HistorianSSLModeVerifyFull.
	SSLMode HistorianSSLMode `yaml:"sslmode,omitempty" json:"sslmode,omitempty"`
	// Port is the Postgres port. Defaults to 5432.
	Port uint16 `yaml:"port,omitempty" json:"port,omitempty"`
}

// Validate returns an error if any required field is missing or any value is invalid.
func (h HistorianConfig) Validate() error {
	required := []struct {
		value  string
		errMsg string
	}{
		{h.Host, "missing required field host"},
		{h.Password, "missing required field password"},
	}

	// Check required fields
	for _, f := range required {
		if f.value == "" {
			return errors.New(f.errMsg)
		}
	}

	// Validate additional fields where possible
	if h.SSLMode != "" && !h.SSLMode.IsValid() {
		return fmt.Errorf("invalid sslmode %q: must be one of require, disable, verify-full", h.SSLMode)
	}

	return nil
}

// WithDefaults returns a copy of h with zero-value optional fields set to their
// documented defaults. Required fields (Host, Password) are left unchanged.
func (h HistorianConfig) WithDefaults() HistorianConfig {
	if h.Port == 0 {
		h.Port = 5432
	}

	if h.Database == "" {
		h.Database = "umh"
	}

	if h.Username == "" {
		h.Username = "umh_owner"
	}

	if h.SSLMode == "" {
		h.SSLMode = HistorianSSLModeRequire
	}

	return h
}
