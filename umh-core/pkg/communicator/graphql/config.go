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

package graphql

import (
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// Configuration errors
var (
	ErrInvalidPort = errors.New("invalid port: must be between 1 and 65535")
)

// ServerConfig holds configuration for the GraphQL server
type ServerConfig struct {
	CORSOrigins []string `json:"cors_origins"`
	Port        int      `json:"port"`
	Debug       bool     `json:"debug"`
}

// DefaultServerConfig returns default configuration values
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:        8090,
		Debug:       false,
		CORSOrigins: []string{"*"},
	}
}

// Validate checks if the configuration is valid
func (c *ServerConfig) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return ErrInvalidPort
	}
	return nil
}

// ConfigAdapter provides a way to convert from external config structures
type ConfigAdapter interface {
	GetPort() int
	GetDebug() bool
	GetCORSOrigins() []string
}

// NewServerConfigFromAdapter creates a ServerConfig from any compatible config
func NewServerConfigFromAdapter(adapter ConfigAdapter) *ServerConfig {
	config := &ServerConfig{
		Port:        adapter.GetPort(),
		Debug:       adapter.GetDebug(),
		CORSOrigins: adapter.GetCORSOrigins(),
	}

	// Apply defaults if needed
	if config.Port == 0 {
		config.Port = 8090
	}
	if len(config.CORSOrigins) == 0 {
		config.CORSOrigins = []string{"*"}
	}

	return config
}

// GraphQLConfigAdapter adapts config.GraphQLConfig to our ConfigAdapter interface
type GraphQLConfigAdapter struct {
	config *config.GraphQLConfig
}

// NewGraphQLConfigAdapter creates an adapter for config.GraphQLConfig
func NewGraphQLConfigAdapter(cfg *config.GraphQLConfig) *GraphQLConfigAdapter {
	return &GraphQLConfigAdapter{config: cfg}
}

// GetPort returns the configured port
func (g *GraphQLConfigAdapter) GetPort() int {
	return g.config.Port
}

// GetDebug returns the debug setting
func (g *GraphQLConfigAdapter) GetDebug() bool {
	return g.config.Debug
}

// GetCORSOrigins returns the CORS origins
func (g *GraphQLConfigAdapter) GetCORSOrigins() []string {
	return g.config.CORSOrigins
}
