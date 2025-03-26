package api_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/helper"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools"
)

func TestApi(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Api Suite")
}

var _ = BeforeSuite(func() {
	By("initializing logging")
	tools.InitLogging()

	By("setting the API_URL environment variable")
	// Set API_URL to the production API URL in order to correctly test the reachability
	// of the production API
	helper.Setenv("API_URL", "https://management.umh.app/api")
})
