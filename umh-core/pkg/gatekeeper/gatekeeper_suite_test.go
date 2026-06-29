package gatekeeper

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
)

func TestGatekeeper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gatekeeper Suite")
}

var _ = BeforeSuite(func() {
	encoding.ChooseEncoder(encoding.EncodingCorev1)
})
