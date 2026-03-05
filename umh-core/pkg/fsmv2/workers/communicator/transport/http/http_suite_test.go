package transport_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHTTPTransport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Transport Suite")
}
