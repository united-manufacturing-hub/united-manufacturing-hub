package subscriber_test

import (
	"github.com/united-manufacturing-hub/ManagementConsole/shared/tools"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTools(t *testing.T) {
	tools.InitLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Subscriber Suite")
}
