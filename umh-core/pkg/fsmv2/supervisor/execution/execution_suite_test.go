package execution_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExecution(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Execution Suite")
}
