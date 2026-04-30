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

var _ = Describe("Dual API support", func() {

	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "validator-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	Describe("IsNewAPIWorkerFile", func() {
		It("detects WorkerBase embed with package qualifier", func() {
			file := filepath.Join(tmpDir, "worker.go")
			Expect(os.WriteFile(file, []byte(`package myworker

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

type MyWorker struct {
	fsmv2.WorkerBase[MyConfig, MyStatus]
}
`), 0o644)).To(Succeed())

			Expect(validator.IsNewAPIWorkerFile(file)).To(BeTrue())
		})

		It("detects WorkerBase embed without package qualifier", func() {
			file := filepath.Join(tmpDir, "worker.go")
			Expect(os.WriteFile(file, []byte(`package myworker

type MyWorker struct {
	WorkerBase[MyConfig, MyStatus]
}
`), 0o644)).To(Succeed())

			Expect(validator.IsNewAPIWorkerFile(file)).To(BeTrue())
		})

		It("returns false for old-API worker without WorkerBase", func() {
			file := filepath.Join(tmpDir, "worker.go")
			Expect(os.WriteFile(file, []byte(`package myworker

type MyWorker struct {
	deps *Dependencies
}

func (w *MyWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return nil, nil
}
`), 0o644)).To(Succeed())

			Expect(validator.IsNewAPIWorkerFile(file)).To(BeFalse())
		})

		It("returns false for non-existent file", func() {
			Expect(validator.IsNewAPIWorkerFile("/nonexistent/worker.go")).To(BeFalse())
		})
	})

	Describe("IsNewAPIWorkerDir", func() {
		It("returns true when worker.go has WorkerBase", func() {
			workerFile := filepath.Join(tmpDir, "worker.go")
			Expect(os.WriteFile(workerFile, []byte(`package myworker

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

type MyWorker struct {
	fsmv2.WorkerBase[MyConfig, MyStatus]
}
`), 0o644)).To(Succeed())

			Expect(validator.IsNewAPIWorkerDir(tmpDir)).To(BeTrue())
		})

		It("returns false when worker.go is missing", func() {
			Expect(validator.IsNewAPIWorkerDir(tmpDir)).To(BeFalse())
		})
	})

	Describe("ValidateShutdownCheckFirst accepts method calls", func() {
		It("accepts method call IsShutdownRequested()", func() {
			workerDir := filepath.Join(tmpDir, "workers", "myworker")
			Expect(os.MkdirAll(workerDir, 0o755)).To(Succeed())

			stateFile := filepath.Join(workerDir, "state_running.go")
			Expect(os.WriteFile(stateFile, []byte(`package myworker

type RunningState struct{}

func (s *RunningState) Next(snapAny any) NextResult {
	snap := snapAny.(*Snapshot)

	if snap.Desired.IsShutdownRequested() {
		return Result(nil, SignalNone, nil, "shutdown")
	}

	return Result(s, SignalNone, nil, "running")
}
`), 0o644)).To(Succeed())

			violations := validator.ValidateShutdownCheckFirst(tmpDir)
			Expect(violations).To(BeEmpty(), "method call IsShutdownRequested() should still be accepted")
		})
	})

	Describe("ValidateNextMethodTypeAssertions accepts ConvertWorkerSnapshot", func() {
		It("accepts ConvertWorkerSnapshot as entry-point type conversion", func() {
			workerDir := filepath.Join(tmpDir, "workers", "myworker")
			Expect(os.MkdirAll(workerDir, 0o755)).To(Succeed())

			stateFile := filepath.Join(workerDir, "state_running.go")
			Expect(os.WriteFile(stateFile, []byte(`package myworker

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

type RunningState struct{}

func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[MyConfig, MyStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Result[any, any](nil, fsmv2.SignalNone, nil, "shutdown", nil)
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "running", nil)
}
`), 0o644)).To(Succeed())

			violations := validator.ValidateNextMethodTypeAssertions(tmpDir)
			Expect(violations).To(BeEmpty(), "ConvertWorkerSnapshot should be accepted as entry-point type conversion")
		})

		It("still accepts ConvertSnapshot for old-API workers", func() {
			workerDir := filepath.Join(tmpDir, "workers", "myworker")
			Expect(os.MkdirAll(workerDir, 0o755)).To(Succeed())

			stateFile := filepath.Join(workerDir, "state_running.go")
			Expect(os.WriteFile(stateFile, []byte(`package myworker

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/helpers"

type RunningState struct{}

func (s *RunningState) Next(snapAny any) NextResult {
	snap := helpers.ConvertSnapshot[Observed, *Desired](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return Result(nil, SignalNone, nil, "shutdown")
	}

	return Result(s, SignalNone, nil, "running")
}
`), 0o644)).To(Succeed())

			violations := validator.ValidateNextMethodTypeAssertions(tmpDir)
			Expect(violations).To(BeEmpty(), "ConvertSnapshot should still be accepted for old-API workers")
		})
	})
})
