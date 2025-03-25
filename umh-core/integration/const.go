package integration_test

// Some example "expected" system behavior constraints
const (
	maxAllocBytes        = 512 * 1024 * 1024 // 512 MB max heap usage
	maxErrorCount        = 0                 // zero error policy
	maxStarvedSeconds    = 0                 // zero starved seconds policy
	maxReconcileTime99th = 80.0              // 99th percentile under 80ms
)
