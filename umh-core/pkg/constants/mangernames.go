package constants

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"

const (
	// Manager name constants
	ContainerManagerName         = logger.ComponentContainerManager + "_" + DefaultManagerName
	BenthosManagerName           = logger.ComponentBenthosManager + "_" + DefaultManagerName
	DataflowcomponentManagerName = logger.ComponentDataFlowComponentManager + DefaultManagerName

	// Instance name constants
	CoreInstanceName = "Core"
)
