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

package backoff

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zaptest"
)

// MockComponent demonstrates how to use BackoffManager in a component.
type MockComponent struct {
	backoffManager *BackoffManager
	failNextNTimes int
	operationCount int
}

func NewMockComponent(maxRetries uint64) *MockComponent {
	logger := zaptest.NewLogger(GinkgoT()).Sugar()

	config := DefaultConfig("mock-component", logger)
	config.MaxRetries = maxRetries

	return &MockComponent{
		backoffManager: NewBackoffManager(config),
		failNextNTimes: 0,
		operationCount: 0,
	}
}

// DoOperation attempts an operation with backoff handling
// Unlike the previous version, we'll make this deterministic by not relying on ShouldSkipOperation
// Instead we'll explicitly check if we've reached our operation count or if we're permanently failed.
func (m *MockComponent) DoOperation(tick uint64) error {
	// If permanently failed, always return permanent error
	if m.backoffManager.IsPermanentlyFailed() {
		return m.backoffManager.GetBackoffError(tick)
	}

	// Track operation attempts
	m.operationCount++

	// Determine if we should succeed or fail
	if m.operationCount <= m.failNextNTimes {
		// Simulate a failure
		err := errors.New("simulated operation failure") //nolint:err113 // Test needs dynamic error
		isPermanent := m.backoffManager.SetError(err, tick)

		if isPermanent {
			return m.backoffManager.GetBackoffError(tick) // Permanent failure
		}

		return m.backoffManager.GetBackoffError(tick) // Temporary failure
	}

	// Success case
	m.backoffManager.Reset()

	return nil
}

func (m *MockComponent) SetFailNextNTimes(n int) {
	m.failNextNTimes = n
	m.operationCount = 0
}

func (m *MockComponent) IsPermanentlyFailed() bool {
	return m.backoffManager.IsPermanentlyFailed()
}

func (m *MockComponent) GetLastError() error {
	return m.backoffManager.GetLastError()
}

// Parent demonstrates a parent component that uses MockComponent.
type MockParent struct {
	component *MockComponent
}

func NewMockParent(component *MockComponent) *MockParent {
	return &MockParent{
		component: component,
	}
}

func (p *MockParent) Execute(tick uint64) error {
	err := p.component.DoOperation(tick)
	if err != nil {
		if IsTemporaryBackoffError(err) {
			// For temporary errors, log but continue
			return nil
		}

		if IsPermanentFailureError(err) {
			// For permanent failures, extract original error for context
			originalErr := ExtractOriginalError(err)
			// Then return error to caller
			return errors.New("component permanently failed: " + originalErr.Error()) //nolint:err113 // Test needs dynamic error
		}

		// For other errors, just return them
		return err
	}

	return nil
}

var _ = Describe("Integration Tests", func() {
	var tick uint64

	BeforeEach(func() {
		tick = 0
	})

	Context("when using BackoffManager in components", func() {
		It("should handle temporary failures with retries", func() {
			mockComponent := NewMockComponent(3)
			mockComponent.SetFailNextNTimes(2) // Fail twice then succeed

			parent := NewMockParent(mockComponent)

			// First attempt - fails (1)
			err := parent.Execute(tick)
			tick++
			Expect(err).ToNot(HaveOccurred(), "Parent should return nil for temporary failures")
			Expect(mockComponent.operationCount).To(Equal(1), "Should have attempted once")

			// Second attempt - fails (2)
			err = parent.Execute(tick)
			tick++
			Expect(err).ToNot(HaveOccurred())
			Expect(mockComponent.operationCount).To(Equal(2), "Should have attempted twice")

			// Third attempt - succeeds
			err = parent.Execute(tick)
			tick++
			Expect(err).ToNot(HaveOccurred())
			Expect(mockComponent.operationCount).To(Equal(3), "Should have attempted three times")
			Expect(mockComponent.IsPermanentlyFailed()).To(BeFalse(), "Should not be permanently failed")
		})

		It("should handle permanent failures", func() {
			// Only allow 1 retry before permanent failure (2 total attempts)
			mockComponent := NewMockComponent(1)
			mockComponent.SetFailNextNTimes(3) // Set to fail more times than max retries

			parent := NewMockParent(mockComponent)

			// First attempt - temporary failure
			err := parent.Execute(tick)
			tick++
			Expect(err).ToNot(HaveOccurred(), "First failure should return nil")
			Expect(mockComponent.IsPermanentlyFailed()).To(BeFalse())

			// Second attempt - triggers permanent failure
			err = parent.Execute(tick)
			tick++
			Expect(err).To(HaveOccurred(), "Should return error for permanent failure")
			Expect(err.Error()).To(ContainSubstring("permanently failed"))
			Expect(mockComponent.IsPermanentlyFailed()).To(BeTrue())

			// Additional attempts - should remain permanently failed
			err = parent.Execute(tick)
			tick++
			Expect(err).To(HaveOccurred(), "Should still return error for permanent failure")
		})

		It("should succeed after failures and reset error state", func() {
			mockComponent := NewMockComponent(3)
			mockComponent.SetFailNextNTimes(1) // Fail once then succeed

			parent := NewMockParent(mockComponent)

			// First attempt - fails
			err := parent.Execute(tick)
			tick++
			Expect(err).ToNot(HaveOccurred())
			Expect(mockComponent.operationCount).To(Equal(1), "Should have attempted once")

			// Second attempt - succeeds
			err = parent.Execute(tick)
			tick++
			Expect(err).ToNot(HaveOccurred())
			Expect(mockComponent.operationCount).To(Equal(2), "Should have attempted twice")
			Expect(mockComponent.GetLastError()).To(Succeed(), "Error should be reset after success")
		})
	})
})
