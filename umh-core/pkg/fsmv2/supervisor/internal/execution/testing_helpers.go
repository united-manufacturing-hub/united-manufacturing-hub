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

package execution

import (
	"context"

	"github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// VerifyActionIdempotency tests that an action is idempotent by executing it
// multiple times and verifying the result is the same as executing it once.
// Recommend 3-5 iterations.
//
// Example:
//
//	It("should be idempotent", func() {
//	    action := &MyAction{...}
//	    execution.VerifyActionIdempotency(action, 3, func() {
//	        // Verify final state here
//	        Expect(fileExists("test.txt")).To(BeTrue())
//	        Expect(fileContent("test.txt")).To(Equal("expected"))
//	    })
//	})
func VerifyActionIdempotency(action fsmv2.Action[any], iterations int, verifyState func()) {
	ctx := context.Background()

	for i := range iterations {
		err := action.Execute(ctx, nil)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Action should succeed on iteration %d", i+1)
	}

	verifyState()
}

// VerifyActionIdempotencyWithSetup tests idempotency with setup/teardown callbacks.
//
// Example:
//
//	It("should be idempotent with cleanup", func() {
//	    execution.VerifyActionIdempotencyWithSetup(
//	        func() { createTempDir() },      // setup
//	        func() { deleteTempDir() },      // teardown
//	        &MyAction{...},                  // action
//	        5,                               // iterations
//	        func() { Expect(...).To(...) },  // verify
//	    )
//	})
func VerifyActionIdempotencyWithSetup(
	setup func(),
	teardown func(),
	action fsmv2.Action[any],
	iterations int,
	verifyState func(),
) {
	setup()

	defer teardown()

	VerifyActionIdempotency(action, iterations, verifyState)
}
