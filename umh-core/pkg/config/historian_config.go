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
	// Timescale holds the TimescaleDB/Postgres connection settings. It is required:
	// the historian section is optional as a whole (FullConfig.Historian is a pointer),
	// but once present it always carries a timescale connection.
	Timescale TimescaleConfig `yaml:"timescale" json:"timescale"`
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
	if h.Timescale == (TimescaleConfig{}) {
		return errors.New("missing required section timescale")
	}

	return h.Timescale.Validate()
}

// ValidateForUpdate validates an edit payload, requiring the timescale section but
// not its password (see TimescaleConfig.ValidateForUpdate).
func (h HistorianConfig) ValidateForUpdate() error {
	if h.Timescale == (TimescaleConfig{}) {
		return errors.New("missing required section timescale")
	}

	return h.Timescale.ValidateForUpdate()
}

// WithDefaults returns a copy of h with defaults applied to its sub-sections. Both
// h and its Timescale are values, so the copy is independent of the receiver and
// callers can redact the reply copy without mutating stored config.
func (h HistorianConfig) WithDefaults() HistorianConfig {
	h.Timescale = h.Timescale.WithDefaults()

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

// ToTemplateMap returns the historian settings as a nested map for template
// rendering, mirroring the `historian.timescale` shape in config.yaml. Bridge
// templates reference the connection as `{{ .historian.timescale.host }}`,
// `{{ .historian.timescale.port }}`, and the other fields. It returns an empty
// map when no timescale section is present.
func (h HistorianConfig) ToTemplateMap() map[string]any {
	if h.Timescale == (TimescaleConfig{}) {
		return map[string]any{}
	}

	return map[string]any{
		"timescale": h.Timescale.ToTemplateMap(),
	}
}

// TimescaleTemplateKeys is the complete set of keys TimescaleConfig.ToTemplateMap
// exposes under {{ .historian.timescale.* }}. It is the template-variable contract:
// bridge templates may reference exactly these keys. Keep it in step with
// ToTemplateMap — the key-set lock test fails if the two diverge.
var TimescaleTemplateKeys = []string{
	"host", "port", "database", "username", "password",
	"sslmode", "sslrootcert", "sslcert", "sslkey",
}

// ToTemplateMap returns the timescale connection settings as a flat map keyed by
// the config.yaml field names, for template rendering. Defaults are applied first
// so templates always see resolved values.
//
// The map is built explicitly, not by marshalling the struct. Bridge templates
// reference every key unconditionally and RenderTemplate runs with
// missingkey=error, so the exposed key set must be total and independent of field
// values. A json:omitempty round-trip would drop an unset optional field (e.g. a
// TLS certificate path left empty under sslmode=require) and fail the render.
func (t TimescaleConfig) ToTemplateMap() map[string]any {
	t = t.WithDefaults()

	return map[string]any{
		"host":        t.Host,
		"port":        t.Port,
		"database":    t.Database,
		"username":    t.Username,
		"password":    t.Password,
		"sslmode":     string(t.SSLMode),
		"sslrootcert": t.SSLRootCert,
		"sslcert":     t.SSLCert,
		"sslkey":      t.SSLKey,
	}
}
