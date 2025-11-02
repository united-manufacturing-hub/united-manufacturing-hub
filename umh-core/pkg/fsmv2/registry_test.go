package fsmv2_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

func TestFsmv2Registry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FSMv2 Registry Suite")
}

var _ = Describe("BaseRegistry", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	Describe("NewBaseRegistry", func() {
		It("should create a non-nil registry", func() {
			registry := fsmv2.NewBaseRegistry(logger)
			Expect(registry).NotTo(BeNil())
		})

		It("should return the logger passed to constructor", func() {
			registry := fsmv2.NewBaseRegistry(logger)
			Expect(registry.GetLogger()).To(Equal(logger))
		})
	})

	Describe("Registry interface compliance", func() {
		It("should implement Registry interface", func() {
			registry := fsmv2.NewBaseRegistry(logger)
			var _ fsmv2.Registry = registry
		})
	})
})
