package ipm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIpm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ipm Suite")
}
