package fsmv2

import "go.uber.org/zap"

// Registry provides access to worker-specific tools for actions.
// All worker registries embed BaseRegistry and extend with worker-specific tools.
type Registry interface {
	GetLogger() *zap.SugaredLogger
}

// BaseRegistry provides common tools for all workers.
// Worker-specific registries should embed this struct.
type BaseRegistry struct {
	logger *zap.SugaredLogger
}

// NewBaseRegistry creates a new base registry with common tools.
func NewBaseRegistry(logger *zap.SugaredLogger) *BaseRegistry {
	if logger == nil {
		panic("NewBaseRegistry: logger cannot be nil")
	}

	return &BaseRegistry{logger: logger}
}

// GetLogger returns the logger for this registry.
func (r *BaseRegistry) GetLogger() *zap.SugaredLogger {
	return r.logger
}
