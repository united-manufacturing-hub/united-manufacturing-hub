package pull_test

import (
	"github.com/united-manufacturing-hub/ManagementConsole/shared/tools"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPull(t *testing.T) {
	tools.InitLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pull Suite")
}
