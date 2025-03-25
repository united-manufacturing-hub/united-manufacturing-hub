package benthos

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBenthos(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Benthos Suite")
}
