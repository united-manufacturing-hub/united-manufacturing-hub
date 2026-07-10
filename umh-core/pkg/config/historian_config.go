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

// ErrHistorianNotConfigured is returned by AtomicEditHistorian when no historian
// section exists to edit.
var ErrHistorianNotConfigured = errors.New("historian is not configured")

// ErrHistorianAlreadyConfigured is returned by AtomicSetHistorian when a historian
// section already exists. Deploy is create-only; callers must use edit to change an
// existing historian, so a retried or misdirected deploy cannot silently overwrite
// connection settings.
var ErrHistorianAlreadyConfigured = errors.New("historian is already configured")

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

// HistorianConfig is the top-level `historian:` section in config.yaml. It groups
// the historian's connection sub-blocks so more backends can be added later without
// flattening: today it holds the TimescaleDB connection under `timescale:`; a
// `grafana:` block is expected to sit alongside it at the same level.
type HistorianConfig struct {
	// Timescale holds the TimescaleDB/Postgres connection settings.
	Timescale *TimescaleConfig `yaml:"timescale,omitempty" json:"timescale,omitempty"`
}

// TimescaleConfig holds the connection settings for the TimescaleDB/Postgres
// historian. It maps to `historian.timescale` in config.yaml.
type TimescaleConfig struct {
	// SSLRootCert is the path inside the container to the CA certificate file.
	SSLRootCert string `yaml:"sslrootcert,omitempty" json:"sslrootcert,omitempty"`
	// SSLCert is the path inside the container to the client certificate file.
	SSLCert string `yaml:"sslcert,omitempty" json:"sslcert,omitempty"`
	// SSLKey is the path inside the container to the client key file.
	SSLKey string `yaml:"sslkey,omitempty" json:"sslkey,omitempty"`
	// Host is the TimescaleDB/Postgres hostname or IP address (required).
	Host string `yaml:"host" json:"host"`
	// Password is the login role password (required). Masked by String() so it
	// never reaches a %v log line; the raw value is still marshalled to YAML/JSON.
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

// Validate returns an error if the historian has no timescale section or that
// section is invalid.
func (h HistorianConfig) Validate() error {
	if h.Timescale == nil {
		return errors.New("missing required section timescale")
	}

	return h.Timescale.Validate()
}

// ValidateForUpdate validates an edit payload, requiring the timescale section but
// not its password (see TimescaleConfig.ValidateForUpdate).
func (h HistorianConfig) ValidateForUpdate() error {
	if h.Timescale == nil {
		return errors.New("missing required section timescale")
	}

	return h.Timescale.ValidateForUpdate()
}

// WithDefaults returns a copy of h with defaults applied to its sub-sections. The
// returned copy owns a fresh Timescale pointer, so callers can redact the reply
// copy without mutating stored config.
func (h HistorianConfig) WithDefaults() HistorianConfig {
	if h.Timescale != nil {
		ts := h.Timescale.WithDefaults()
		h.Timescale = &ts
	}

	return h
}

// Validate returns an error if any required field is missing or any value is invalid.
func (t TimescaleConfig) Validate() error {
	required := []struct {
		value  string
		errMsg string
	}{
		{t.Host, "missing required field host"},
		{t.Password, "missing required field password"},
	}

	// Check required fields
	for _, f := range required {
		if f.value == "" {
			return errors.New(f.errMsg)
		}
	}

	return t.validateSSL()
}

// ValidateForUpdate validates an edit payload. Unlike Validate it does not require
// Password: get-historian never returns the stored password, so the Management
// Console cannot resend it, and an empty password on edit means "keep the existing
// one" (see AtomicEditHistorian). Host is still required.
func (t TimescaleConfig) ValidateForUpdate() error {
	if t.Host == "" {
		return errors.New("missing required field host")
	}

	return t.validateSSL()
}

// hasTLSCerts reports whether any TLS certificate path is configured.
func (t TimescaleConfig) hasTLSCerts() bool {
	return t.SSLRootCert != "" || t.SSLCert != "" || t.SSLKey != ""
}

// validateSSL checks the sslmode value and its consistency with the configured
// certificates. Certificates only take effect under verify-full: require encrypts
// but skips server-certificate verification, and disable skips TLS entirely, so
// supplying a CA or client certificate with either mode would silently leave the
// connection open to a man-in-the-middle. Rather than pick a default that could be
// wrong, reject the contradiction and make the operator choose verify-full.
func (t TimescaleConfig) validateSSL() error {
	if t.SSLMode != "" && !t.SSLMode.IsValid() {
		return fmt.Errorf("invalid sslmode %q: must be one of require, disable, verify-full", t.SSLMode)
	}

	if t.hasTLSCerts() && t.SSLMode != HistorianSSLModeVerifyFull {
		mode := string(t.SSLMode)
		if mode == "" {
			mode = "unset (defaults to require)"
		}

		return fmt.Errorf("tls certificate paths are set but sslmode is %s; certificates only verify the server with sslmode %q", mode, HistorianSSLModeVerifyFull)
	}

	return nil
}

// WithDefaults returns a copy of t with zero-value optional fields set to their
// documented defaults. Required fields (Host, Password) are left unchanged.
func (t TimescaleConfig) WithDefaults() TimescaleConfig {
	if t.Port == 0 {
		t.Port = 5432
	}

	if t.Database == "" {
		t.Database = "umh"
	}

	if t.Username == "" {
		t.Username = "umh_owner"
	}

	if t.SSLMode == "" {
		t.SSLMode = HistorianSSLModeRequire
	}

	return t
}

// String masks Password so the timescale config is safe to log with %v.
// It only affects logging; YAML/JSON marshalling still emits the real password.
func (t TimescaleConfig) String() string {
	if t.Password != "" {
		t.Password = "[REDACTED]"
	}

	type redacted TimescaleConfig
	return fmt.Sprintf("%+v", redacted(t))
}
