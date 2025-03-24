package control

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestControl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Control Suite")
}
