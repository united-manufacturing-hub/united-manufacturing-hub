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

package helper

import (
	"os"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo is designed to be used with dot imports
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega is designed to be used with dot imports
)

func Setenv(env, value string) {
	// this looks weird, but the Ginkgo documentation explicitly quote this pattern
	// https://onsi.github.io/ginkgo/#cleaning-up-our-cleanup-code-defercleanup
	// or look at the function documentation for DeferCleanup
	DeferCleanup(os.Setenv, env, os.Getenv("DEMO_MODE"))
	Expect(os.Setenv(env, value)).To(Succeed())
}

func IsTest() bool {
	// Analyze the call stack to determine if the caller is a test
	// If it is a test the callstack will have files with _test.go
	pcs := make([]uintptr, 10) // adjust the depth as necessary
	runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs)

	// Iterate through the call stack frames.
	for frame, more := frames.Next(); more; frame, more = frames.Next() {
		if strings.HasSuffix(frame.File, "_test.go") {
			return true
		}
	}

	return false
}
