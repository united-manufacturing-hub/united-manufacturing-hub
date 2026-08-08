// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
)

func TestMain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Suite")
}

var _ = Describe("bringUpFSMv2", func() {
	// bringUpFSMv2 is the bring-up decision main() consults. It is a pure
	// function of the backend configuration, so it is asserted directly here
	// rather than by running main(). The keep-the-legacy knob
	// (UseFSMv2Transport) no longer routes anywhere: the legacy backend path
	// was deleted, so FSMv2 is the only bring-up path.

	It("brings up FSMv2 when credentials are present, even with the legacy transport switch off", func() {
		cfg := config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:            "http://fsmv2.invalid:9999",
					AuthToken:         "test-token",
					UseFSMv2Transport: false,
				},
			},
		}

		Expect(bringUpFSMv2(&cfg)).To(BeTrue())
	})

	It("does not bring up FSMv2 when credentials are absent", func() {
		cfg := config.FullConfig{}

		Expect(bringUpFSMv2(&cfg)).To(BeFalse())
	})

	It("does not bring up FSMv2 when only API_URL is set", func() {
		cfg := config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL: "http://fsmv2.invalid:9999",
				},
			},
		}

		// The && credential contract must reject partial state; an operator with
		// one field empty should not run without a token.
		Expect(bringUpFSMv2(&cfg)).To(BeFalse())
	})

	It("does not bring up FSMv2 when only AUTH_TOKEN is set", func() {
		cfg := config.FullConfig{
			Agent: config.AgentConfig{
				CommunicatorConfig: config.CommunicatorConfig{
					AuthToken: "test-token",
				},
			},
		}

		Expect(bringUpFSMv2(&cfg)).To(BeFalse())
	})
})

var _ = Describe("counting historian bridges", func() {
	historianBridge := func() config.ProtocolConverterConfig {
		return config.ProtocolConverterConfig{
			ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
							Protocol: dataflowcomponentserviceconfig.HistorianDestinationProtocol,
						},
					},
				},
			},
		}
	}

	nonHistorianBridge := func() config.ProtocolConverterConfig {
		return config.ProtocolConverterConfig{
			ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
							Protocol: "kafka",
						},
					},
				},
			},
		}
	}

	It("counts only bridges writing to the historian", func() {
		cfg := config.FullConfig{
			ProtocolConverter: []config.ProtocolConverterConfig{
				historianBridge(),
				nonHistorianBridge(),
				historianBridge(),
			},
		}

		Expect(countHistorianBridges(cfg)).To(Equal(2))
	})

	It("returns zero when no bridges write to the historian", func() {
		cfg := config.FullConfig{
			ProtocolConverter: []config.ProtocolConverterConfig{
				nonHistorianBridge(),
			},
		}

		Expect(countHistorianBridges(cfg)).To(Equal(0))
	})
})
