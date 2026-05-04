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

package validator_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/validator"
)

var _ = Describe("ValidateActionStructs mutation detection", func() {

	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "validator-action-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("does NOT flag config fields that are only read in Execute (regression: MaxFailures, FailureCycles)", func() {
		file := filepath.Join(tmpDir, "workers", "myworker", "action", "connect.go")
		Expect(os.MkdirAll(filepath.Dir(file), 0o755)).To(Succeed())
		Expect(os.WriteFile(file, []byte(`package action

import "context"

type ConnectAction struct {
	ShouldFail    bool
	MaxFailures   int
	FailureCycles int
}

func (a *ConnectAction) Execute(ctx context.Context, deps any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_ = a.MaxFailures
	_ = a.FailureCycles
	return nil
}

func (a *ConnectAction) String() string { return "connect" }
func (a *ConnectAction) Name() string   { return "connect" }
`), 0o644)).To(Succeed())

		violations := validator.ValidateActionStructs(tmpDir)
		Expect(violations).To(BeEmpty(),
			"fields only READ in Execute should not be flagged as mutable state")
	})

	It("flags a counter field that is incremented with ++ in Execute", func() {
		file := filepath.Join(tmpDir, "workers", "myworker", "action", "retry.go")
		Expect(os.MkdirAll(filepath.Dir(file), 0o755)).To(Succeed())
		Expect(os.WriteFile(file, []byte(`package action

import "context"

type RetryAction struct {
	counter int
}

func (a *RetryAction) Execute(ctx context.Context, deps any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	a.counter++
	return nil
}

func (a *RetryAction) String() string { return "retry" }
func (a *RetryAction) Name() string   { return "retry" }
`), 0o644)).To(Succeed())

		violations := validator.ValidateActionStructs(tmpDir)
		Expect(violations).To(HaveLen(1))
		Expect(violations[0].Type).To(Equal("STATELESS_ACTION"))
		Expect(violations[0].Message).To(ContainSubstring("counter"))
	})

	It("flags a field mutated via compound assignment (+=)", func() {
		file := filepath.Join(tmpDir, "workers", "myworker", "action", "accumulate.go")
		Expect(os.MkdirAll(filepath.Dir(file), 0o755)).To(Succeed())
		Expect(os.WriteFile(file, []byte(`package action

import "context"

type AccumulateAction struct {
	attempts int
}

func (a *AccumulateAction) Execute(ctx context.Context, deps any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	a.attempts += 1
	return nil
}

func (a *AccumulateAction) String() string { return "accumulate" }
func (a *AccumulateAction) Name() string   { return "accumulate" }
`), 0o644)).To(Succeed())

		violations := validator.ValidateActionStructs(tmpDir)
		Expect(violations).To(HaveLen(1))
		Expect(violations[0].Type).To(Equal("STATELESS_ACTION"))
		Expect(violations[0].Message).To(ContainSubstring("attempts"))
	})
})
