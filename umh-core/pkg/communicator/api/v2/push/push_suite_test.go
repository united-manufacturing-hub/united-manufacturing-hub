package push_test

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPush(t *testing.T) {
	tools.InitLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Push Suite")
}
