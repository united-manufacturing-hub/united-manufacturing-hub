// Copyright 2025 UMH Systems GmbH
package execution_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Action Idempotency Pattern (I10)", func() {
	Describe("Example idempotent action", func() {
		It("should demonstrate idempotency pattern", func() {
			action := &exampleIdempotentAction{value: 42}

			VerifyActionIdempotency(action, 5, func() {
				Expect(action.executionCount).To(BeNumerically(">=", 5))
				Expect(action.finalValue).To(Equal(42))
			})
		})
	})

	Describe("Counter-example: non-idempotent action", func() {
		It("should show why non-idempotent actions fail", func() {
			action := &exampleNonIdempotentAction{}

			for i := 0; i < 3; i++ {
				_ = action.Execute(context.Background())
			}

			Expect(action.counter).To(Equal(3), "Non-idempotent action increments on each call")
		})
	})
})

type exampleIdempotentAction struct {
	value          int
	executionCount int
	finalValue     int
}

func (a *exampleIdempotentAction) Execute(ctx context.Context) error {
	a.executionCount++

	a.finalValue = a.value

	return nil
}

func (a *exampleIdempotentAction) Name() string {
	return "ExampleIdempotentAction"
}

type exampleNonIdempotentAction struct {
	counter int
}

func (a *exampleNonIdempotentAction) Execute(ctx context.Context) error {
	a.counter++

	return nil
}

func (a *exampleNonIdempotentAction) Name() string {
	return "ExampleNonIdempotentAction"
}
