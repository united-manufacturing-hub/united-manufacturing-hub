package watchdog_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
)

func TestShared(t *testing.T) {
	tools.InitLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Watchdog Suite")
}
