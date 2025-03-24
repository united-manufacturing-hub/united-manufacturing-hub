package starvationchecker

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStarvation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Starvation Suite")
}
