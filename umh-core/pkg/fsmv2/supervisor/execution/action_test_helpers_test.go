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

package execution_test

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"
)

// VerifyActionIdempotency is a convenience wrapper for execution.VerifyActionIdempotency
// that allows tests in this package to use it without the package prefix.
//
// Deprecated: Use execution.VerifyActionIdempotency directly.
func VerifyActionIdempotency(action fsmv2.Action, iterations int, verifyState func()) {
	execution.VerifyActionIdempotency(action, iterations, verifyState)
}

// VerifyActionIdempotencyWithSetup is a convenience wrapper for execution.VerifyActionIdempotencyWithSetup
// that allows tests in this package to use it without the package prefix.
//
// Deprecated: Use execution.VerifyActionIdempotencyWithSetup directly.
func VerifyActionIdempotencyWithSetup(
	setup func(),
	teardown func(),
	action fsmv2.Action,
	iterations int,
	verifyState func(),
) {
	execution.VerifyActionIdempotencyWithSetup(setup, teardown, action, iterations, verifyState)
}
