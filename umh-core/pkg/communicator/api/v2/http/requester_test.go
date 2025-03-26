package http_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/helper"
)

var _ = Describe("Requester", func() {
	BeforeEach(func() {
		By("setting the DEMO_MODE environment variable")
		// Set DEMO_MODE to true in order to use the fake kube clientset in this suite
		// instead of a real one
		helper.Setenv("DEMO_MODE", "true")

		By("setting the API_URL environment variable")
		// Set API_URL to the production API URL in order for the gock library to correctly
		// intercept the requests to the production API done by the code under test
		helper.Setenv("API_URL", "https://management.umh.app/api")
	})

	// The tests in this context are ported from the old requester_test.go file
	Context("Requester", func() {
		var header map[string]string
		var cookies map[string]string
		var data map[string]string

		BeforeEach(func() {
			header = map[string]string{
				"Content-Type": "application/json",
			}
			cookies = map[string]string{
				"test": "test",
			}
			data = map[string]string{
				"test": "test",
			}
		})

		Context("GetRequest", func() {
			It("should return error for non 200 response", func() {
				_, err, _ := http.GetRequest[any](context.Background(), http.LoginEndpoint, header, &cookies, false)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("PostRequest", func() {
			It("should return error for non 200 response", func() {
				_, err, _ := http.PostRequest[any](context.Background(), http.LoginEndpoint, &data, header, &cookies, false)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
