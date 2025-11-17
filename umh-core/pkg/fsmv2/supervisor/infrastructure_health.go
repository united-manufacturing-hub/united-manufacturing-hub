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

package supervisor

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"
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
	backoff       *execution.ExponentialBackoff
	maxAttempts   int
	attemptWindow time.Duration
}

func NewInfrastructureHealthChecker(maxAttempts int, attemptWindow time.Duration) *InfrastructureHealthChecker {
	return &InfrastructureHealthChecker{
		backoff:       execution.NewExponentialBackoff(1*time.Second, 60*time.Second),
		maxAttempts:   maxAttempts,
		attemptWindow: attemptWindow,
	}
}

func (h *InfrastructureHealthChecker) CheckChildConsistency(children map[string]SupervisorInterface) error {
	for name, child := range children {
		if child == nil {
			continue
		}

		// Type assert to access circuitOpen field (internal to package)
		// We use interface{} because Supervisor has different type parameters per child
		type circuitChecker interface {
			isCircuitOpen() bool
		}

		if checker, ok := child.(circuitChecker); ok && checker.isCircuitOpen() {
			return &ChildHealthError{ChildName: name}
		}
	}

	return nil
}
