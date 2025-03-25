package s6

import (
	"fmt"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/metrics"
)

const (
	baseS6Dir = constants.S6BaseDir
)

// S6Manager implements FSM management for S6 services.
type S6Manager struct {
	*public_fsm.BaseFSMManager[config.S6FSMConfig]
}

// NewS6Manager creates a new S6Manager
// The name is used to identify the manager in logs, as other components that leverage s6 will sue their own instance of this manager
func NewS6Manager(name string) *S6Manager {

	managerName := fmt.Sprintf("%s%s", logger.ComponentS6Manager, name)

	baseManager := public_fsm.NewBaseFSMManager[config.S6FSMConfig](
		managerName,
		baseS6Dir,
		// Extract S6 configs from full config
		func(fullConfig config.FullConfig) ([]config.S6FSMConfig, error) {
			return fullConfig.Services, nil
		},
		// Get name from S6 config
		func(cfg config.S6FSMConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from S6 config
		func(cfg config.S6FSMConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create S6 instance from config
		func(cfg config.S6FSMConfig) (public_fsm.FSMInstance, error) {
			return NewS6Instance(baseS6Dir, cfg)
		},
		// Compare S6 configs
		func(instance public_fsm.FSMInstance, cfg config.S6FSMConfig) (bool, error) {
			s6Instance, ok := instance.(*S6Instance)
			if !ok {
				return false, fmt.Errorf("instance is not an S6Instance")
			}
			return s6Instance.config.S6ServiceConfig.Equal(cfg.S6ServiceConfig), nil
		},
		// Set S6 config
		func(instance public_fsm.FSMInstance, cfg config.S6FSMConfig) error {
			s6Instance, ok := instance.(*S6Instance)
			if !ok {
				return fmt.Errorf("instance is not an S6Instance")
			}
			s6Instance.config.S6ServiceConfig = cfg.S6ServiceConfig
			return nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentS6Manager, name)

	return &S6Manager{
		BaseFSMManager: baseManager,
	}
}
