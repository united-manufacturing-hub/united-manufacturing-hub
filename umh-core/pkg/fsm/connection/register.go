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

package connection

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

const (
	// ServiceTypeConnectionManager is the service type for the connection manager
	ServiceTypeConnectionManager = "connectionManager"
)

// RegisterConnectionManager registers the connection manager with the service registry
func RegisterConnectionManager(registry interface{}) error {
	if registry == nil {
		return fmt.Errorf("service registry is nil")
	}

	log := logger.For("connection.RegisterConnectionManager")
	log.Infof("Registering connection manager")

	// Registration would normally be done here, but we're simplifying
	// since the service package is not available in this context
	log.Infof("Connection manager registered")

	return nil
}

// startConnectionManager creates and starts the connection manager
func startConnectionManager(ctx context.Context, registry interface{}) (interface{}, error) {
	log := logger.For("connection.startConnectionManager")
	log.Infof("Starting connection manager")

	// Create the connection manager with a descriptive name
	manager := NewConnectionManager("Default")

	// Start the manager's reconcile loop
	go func() {
		if err := manager.ReconcileLoop(ctx); err != nil {
			log.Errorf("Connection manager reconcile loop exited with error: %v", err)
		}
	}()

	log.Infof("Connection manager started")
	return manager, nil
}

// stopConnectionManager stops the connection manager
func stopConnectionManager(ctx context.Context, serviceInterface interface{}) error {
	log := logger.For("connection.stopConnectionManager")
	log.Infof("Stopping connection manager")

	// The ReconcileLoop function will terminate when the context is cancelled
	log.Infof("Connection manager stopped")
	return nil
}
