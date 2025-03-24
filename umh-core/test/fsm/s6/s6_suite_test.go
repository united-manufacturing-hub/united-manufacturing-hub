package s6_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestS6(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "S6 Test Suite")
}
