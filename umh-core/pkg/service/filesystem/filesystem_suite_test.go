package filesystem_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBufferedService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BufferedService Suite")
}
