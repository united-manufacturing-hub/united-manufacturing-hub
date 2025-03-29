package constants

import "time"

const (
	// defaultTickerTime is the interval between reconciliation cycles.
	// This value balances responsiveness with resource utilization:
	// - Too small: could mean that the managers do not have enough time to complete their work
	// - Too high: Delayed response to configuration changes
	DefaultTickerTime = 100 * time.Millisecond

	// starvationThreshold defines when to consider the control loop starved.
	// If no reconciliation has happened for this duration, the starvation
	// detector will log warnings and record metrics.
	// Starvation will take place for example when adding hundreds of new services
	// at once.
	StarvationThreshold = 15 * time.Second

	// DefaultManagerName is the default name for a manager.
	DefaultManagerName = "Core"

	// DefaultInstanceName is the default name for an instance.
	DefaultInstanceName = "Core"
)
