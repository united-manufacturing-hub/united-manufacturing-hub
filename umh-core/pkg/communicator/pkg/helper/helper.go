package helper

import (
	"os"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
