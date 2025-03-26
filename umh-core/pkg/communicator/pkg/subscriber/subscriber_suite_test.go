package subscriber_test

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTools(t *testing.T) {
	tools.InitLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Subscriber Suite")
}
