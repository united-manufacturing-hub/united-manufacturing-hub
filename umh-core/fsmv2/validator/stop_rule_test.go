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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/validator"
)

const runningStateCheckingOnlyShutdown = `package state

func (s *RunningState) Next(snapAny any) any {
	snap := convert(snapAny)

	if snap.IsShutdownRequested {
		return stop()
	}

	return s
}
`

const runningStateCheckingShouldStop = `package state

func (s *RunningState) Next(snapAny any) any {
	snap := convert(snapAny)

	if snap.ShouldStop() {
		return stop()
	}

	return s
}
`

// writeStateFile writes source as workers/demo/state/<fileName> under a new
// temporary base directory and returns that base directory.
func writeStateFile(fileName string, source string) string {
	baseDir := GinkgoT().TempDir()
	stateDir := filepath.Join(baseDir, "workers", "demo", "state")
	Expect(os.MkdirAll(stateDir, 0o755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(stateDir, fileName), []byte(source), 0o644)).To(Succeed())

	return baseDir
}

var _ = Describe("ValidateShutdownCheckFirst", func() {
	It("rejects an active state whose first check reacts only to a shutdown", func() {
		baseDir := writeStateFile("state_running.go", runningStateCheckingOnlyShutdown)

		violations := validator.ValidateShutdownCheckFirst(baseDir)

		Expect(violations).To(HaveLen(1))
		Expect(violations[0].Type).To(Equal("SHUTDOWN_CHECK_NOT_FIRST"))
	})

	It("accepts an active state whose first check is ShouldStop()", func() {
		baseDir := writeStateFile("state_running.go", runningStateCheckingShouldStop)

		Expect(validator.ValidateShutdownCheckFirst(baseDir)).To(BeEmpty())
	})
})
