package integration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Standard Ginkgo bootstrap
func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UMH Integration Suite")
}
