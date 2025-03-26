package push_test

import (
	"testing"

	"github.com/united-manufacturing-hub/ManagementConsole/shared/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPush(t *testing.T) {
	tools.InitLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Push Suite")
}
