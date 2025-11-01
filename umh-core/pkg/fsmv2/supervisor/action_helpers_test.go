// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// VerifyActionIdempotency tests that an action is idempotent by executing it
// multiple times and verifying the result is the same as executing it once.
//
// Parameters:
//   - action: The action to test
//   - iterations: Number of times to execute (recommend 3-5)
//   - verifyState: Function that checks the final state is correct
//
// Example usage:
//
//	It("should be idempotent", func() {
//	    action := &MyAction{...}
//	    VerifyActionIdempotency(action, 3, func() {
//	        // Verify final state here
//	        Expect(fileExists("test.txt")).To(BeTrue())
//	        Expect(fileContent("test.txt")).To(Equal("expected"))
//	    })
//	})
func VerifyActionIdempotency(action fsmv2.Action, iterations int, verifyState func()) {
	ctx := context.Background()

	for i := 0; i < iterations; i++ {
		err := action.Execute(ctx)
		Expect(err).ToNot(HaveOccurred(), "Action should succeed on iteration %d", i+1)
	}

	verifyState()
}

// VerifyActionIdempotencyWithSetup tests idempotency with setup/teardown.
//
// Parameters:
//   - setup: Called once before all executions to prepare test state
//   - teardown: Called once after all executions to clean up
//   - action: The action to test
//   - iterations: Number of times to execute
//   - verifyState: Function that checks the final state
//
// Example:
//
//	It("should be idempotent with cleanup", func() {
//	    VerifyActionIdempotencyWithSetup(
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
	action fsmv2.Action,
	iterations int,
	verifyState func(),
) {
	setup()

	defer teardown()

	VerifyActionIdempotency(action, iterations, verifyState)
}
