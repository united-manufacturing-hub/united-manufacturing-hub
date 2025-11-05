package supervisor

import (
	"fmt"
	"time"
)

const (
	DefaultMaxInfraRecoveryAttempts = 5
	DefaultRecoveryAttemptWindow    = 5 * time.Minute
)

type ChildHealthError struct {
	ChildName string
	Err       error
}

func (e *ChildHealthError) Error() string {
	return fmt.Sprintf("child %s unhealthy: %v", e.ChildName, e.Err)
}

type InfrastructureHealthChecker struct {
	backoff       *ExponentialBackoff
	maxAttempts   int
	attemptWindow time.Duration
}

func NewInfrastructureHealthChecker(maxAttempts int, attemptWindow time.Duration) *InfrastructureHealthChecker {
	return &InfrastructureHealthChecker{
		backoff:       NewExponentialBackoff(1*time.Second, 60*time.Second),
		maxAttempts:   maxAttempts,
		attemptWindow: attemptWindow,
	}
}

func (h *InfrastructureHealthChecker) CheckChildConsistency(children map[string]*Supervisor) error {
	for name, child := range children {
		if child == nil {
			continue
		}
		if child.circuitOpen {
			return &ChildHealthError{ChildName: name}
		}
	}
	return nil
}
