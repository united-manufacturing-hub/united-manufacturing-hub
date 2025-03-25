package mgmtconfig_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/tools"
)

func TestMgmtconfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mgmtconfig Suite")
}

var _ = BeforeSuite(func() {
	By("initializing logging")
	tools.InitLogging()

})
