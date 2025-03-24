package logger

// Component name constants for standardized logging
const (
	// Core components
	ComponentCore              = "Core"
	ComponentControlLoop       = "ControlLoop"
	ComponentStarvationChecker = "StarveCheck"

	// Manager components
	ComponentS6Manager      = "S6Manager"
	ComponentBenthosManager = "BenthosManager"

	// FSM components
	ComponentBaseFSM    = "BaseFSM"
	ComponentS6Instance = "S6Instance"

	// Service components
	ComponentS6Service      = "S6Service"
	ComponentBenthosService = "BenthosService"

	// Configuration
	ComponentConfigManager = "ConfigManager"
)
