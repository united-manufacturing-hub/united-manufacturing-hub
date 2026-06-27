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

package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"go.uber.org/zap"
)

var _ = Describe("buildProtocolConverterAsDfc", func() {
	log := zap.NewNop().Sugar()

	It("emits Dfc.Bridge with InputType (wrapped) and OutputType set for a bidirectional PC", func() {
		snap := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{"input": map[string]any{"http_client": map[string]any{"url": "http://x"}}},
						},
					},
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "kafka"},
					},
				},
			},
		}
		instance := fsm.FSMInstanceSnapshot{ID: "pc-bidi", CurrentState: protocolconverter.OperationalStateActive, DesiredState: protocolconverter.OperationalStateActive, LastObservedState: snap}

		dfc, err := buildProtocolConverterAsDfc(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.Bridge).NotTo(BeNil(), "configured PC must populate Bridge")
		Expect(dfc.Bridge.InputType).To(Equal("http_client"), "InputType = wrapped read plugin (descent)")
		Expect(dfc.Bridge.OutputType).To(Equal("kafka"), "OutputType = write Destination.Protocol")
		Expect(dfc.Bridge.DataContract).To(BeEmpty(), "PCs have no data contract")
		Expect(dfc.IsInitialized).To(BeTrue(), "populated read input means IsInitialized")
	})

	It("leaves Dfc.Bridge nil for an uninitialized PC (no protocol id to emit)", func() {
		snap := &protocolconverter.ProtocolConverterObservedStateSnapshot{}
		instance := fsm.FSMInstanceSnapshot{ID: "pc-uninit", CurrentState: protocolconverter.OperationalStateActive, DesiredState: protocolconverter.OperationalStateActive, LastObservedState: snap}

		dfc, err := buildProtocolConverterAsDfc(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.Bridge).To(BeNil(), "uninitialized PC emits no Bridge (preserves prior wire behavior — empty bridge{} would serialize on the wire since DfcBridgeInfo fields lack omitempty)")
		Expect(dfc.IsInitialized).To(BeFalse())
	})
})
