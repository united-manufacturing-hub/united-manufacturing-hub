package s6

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_shared"
)

// Service defines the interface for interacting with S6 services.
type Service interface {
	// Create creates the service with specific configuration
	Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) error
	// Remove removes the service directory structure
	Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Start starts the service
	Start(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Stop stops the service
	Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Restart restarts the service
	Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Status gets the current status of the service
	Status(ctx context.Context, servicePath string, fsService filesystem.Service) (s6_shared.ServiceInfo, error)
	// ExitHistory gets the exit history of the service
	ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]s6_shared.ExitEvent, error)
	// ServiceExists checks if the service directory exists
	ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
	// GetConfig gets the actual service config from s6
	GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (s6serviceconfig.S6ServiceConfig, error)
	// CleanS6ServiceDirectory cleans the S6 service directory, removing non-standard services
	CleanS6ServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error
	// GetS6ConfigFile retrieves a config file for a service
	GetS6ConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error)
	// ForceRemove removes a service from the S6 manager
	ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// GetLogs gets the logs of the service
	GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]s6_shared.LogEntry, error)
	// EnsureSupervision checks if the supervise directory exists for a service and notifies
	// s6-svscan if it doesn't, to trigger supervision setup.
	// Returns true if supervise directory exists (ready for supervision), false otherwise.
	EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
}
