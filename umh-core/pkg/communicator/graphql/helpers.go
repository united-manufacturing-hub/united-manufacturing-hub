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
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"go.uber.org/zap"
)

// StartGraphQLServer is a convenience function for main.go to start the GraphQL server
// It handles the configuration conversion and server creation.
func StartGraphQLServer(
	resolver *Resolver,
	cfg *config.GraphQLConfig,
	logger *zap.SugaredLogger,
) (*Server, error) {
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil // Server not enabled is not an error condition, no server instance needed
	}

	// Convert config
	adapter := NewGraphQLConfigAdapter(cfg)
	serverConfig := NewServerConfigFromAdapter(adapter)

	// Create and start server
	server, err := NewServer(resolver, serverConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL server: %w", err)
	}

	// Start the server
	if err := server.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start GraphQL server: %w", err)
	}

	return server, nil
}
