package helper

import (
	"os"

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
