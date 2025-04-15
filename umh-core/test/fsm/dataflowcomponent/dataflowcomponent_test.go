package dataflowcomponent_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

func SetupDataflowComponentInstanceForTests(serviceName string, desiredState string) (*dataflowcomponentfsm.DataflowComponentInstance, *dataflowcomponentsvc.MockDataFlowComponentService, config.DataFlowComponentConfig) {
	cfg := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            serviceName,
			DesiredFSMState: desiredState,
		},
		DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
			BenthosConfig: dataflowcomponentconfig.BenthosConfig{
				Input: map[string]interface{}{
					"type": "file",
					"path": "/tmp/dataflowcomponent.log",
				},
			},
		},
	}
	inst := dataflowcomponentfsm.NewDataflowComponentInstance("/dev/null", cfg)
	mockService := dataflowcomponentsvc.NewMockDataFlowComponentService()
	return inst, mockService, cfg
}

var _ = Describe("DataFlowComponent", func() {
	var (
		serviceName string
		manager     *dataflowcomponentfsm.DataflowComponentManager
		s6BaseDir   string
		mockConfig  config.DataFlowComponentConfig
		instance    *dataflowcomponentfsm.DataflowComponentInstance
	)
	BeforeEach(func() {
		serviceName = "test-dataflowcomponent"
		s6BaseDir = "/tmp/s6-test"
		mockConfig = config.DataFlowComponentConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            serviceName,
				DesiredFSMState: dataflowcomponentfsm.OperationalStateStopped,
			},
			DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
				BenthosConfig: dataflowcomponentconfig.BenthosConfig{
					Input: map[string]interface{}{
						"type": "file",
						"path": "/tmp/dataflowcomponent.log",
					},
				},
			},
		}
		manager = dataflowcomponentfsm.NewDataflowComponentManager(serviceName)
		instance = dataflowcomponentfsm.NewDataflowComponentInstance(s6BaseDir, mockConfig)
	})
	Context("DataflowComponent Base Tests", func() {
		It("should create a new DataflowComponentManager", func() {
			Expect(manager).NotTo(BeNil())
		})

		It("should add a new DataflowComponentInstance to the manager", func() {
			Expect(instance).NotTo(BeNil())
		})
	})

	Context("DataflowComponent Instance Tests", Ordered, func() {
		It("should get the current state of the instance", func() {
			Expect(instance.GetCurrentFSMState()).To(Equal("to_be_created"))
		})

		It("should get the desired state of the instance", func() {
			Expect(instance.GetDesiredFSMState()).To(Equal(dataflowcomponentfsm.OperationalStateStopped))
		})

		It("should Reconcile", func() {
			Expect(instance.Reconcile(context.Background(), filesystem.NewMockFileSystem(), 1)).To(Succeed())
		})

	})

	Context("Basic State Transitions", func() {
		It("should transition from Stopped to Starting when activated", func() {
			// 1. Initially, the instance is "to_be_created"
			//    Let's do a short path: to_be_created => creating => stopped

		})
	})
})
