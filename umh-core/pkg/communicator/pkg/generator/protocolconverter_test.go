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
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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

	It("read-only PC emits Bridge with InputType set, OutputType empty", func() {
		snap := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{"input": map[string]any{"modbus": map[string]any{"address": "1"}}},
						},
					},
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{},
				},
			},
		}
		instance := fsm.FSMInstanceSnapshot{ID: "pc-ro", CurrentState: protocolconverter.OperationalStateActive, DesiredState: protocolconverter.OperationalStateActive, LastObservedState: snap}

		dfc, err := buildProtocolConverterAsDfc(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.Bridge).NotTo(BeNil())
		Expect(dfc.Bridge.InputType).To(Equal("modbus"))
		Expect(dfc.Bridge.OutputType).To(BeEmpty(), "no write configured")
	})

	It("write-only PC emits Bridge with OutputType set, InputType empty (gate is not read-biased)", func() {
		snap := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "questdb"},
					},
				},
			},
		}
		instance := fsm.FSMInstanceSnapshot{ID: "pc-wo", CurrentState: protocolconverter.OperationalStateActive, DesiredState: protocolconverter.OperationalStateActive, LastObservedState: snap}

		dfc, err := buildProtocolConverterAsDfc(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.Bridge).NotTo(BeNil(), "write-only PC has a protocol id to emit (OutputType)")
		Expect(dfc.Bridge.InputType).To(BeEmpty(), "no read configured")
		Expect(dfc.Bridge.OutputType).To(Equal("questdb"))
		Expect(dfc.IsInitialized).To(BeFalse(), "isInitialized is read-biased; write-only stays false until ENG-5251")
	})

	It("Code-without-Protocol emits OutputType empty (diverges from HasOutput)", func() {
		snap := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{"input": map[string]any{"mqtt": map[string]any{}}},
						},
					},
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Code: "kafka:\n  topic: t"},
					},
				},
			},
		}
		instance := fsm.FSMInstanceSnapshot{ID: "pc-code", CurrentState: protocolconverter.OperationalStateActive, DesiredState: protocolconverter.OperationalStateActive, LastObservedState: snap}

		dfc, err := buildProtocolConverterAsDfc(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.Bridge).NotTo(BeNil(), "read configured → InputType non-empty → Bridge populated")
		Expect(dfc.Bridge.InputType).To(Equal("mqtt"))
		Expect(dfc.Bridge.OutputType).To(BeEmpty(), "OutputType is Destination.Protocol, which is empty; HasOutput()=true but the gate uses Protocol, not HasOutput")
	})
})

func bidirectionalSnap() *protocolconverter.ProtocolConverterObservedStateSnapshot {
	return &protocolconverter.ProtocolConverterObservedStateSnapshot{
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
}

func toInstance(id string, snap *protocolconverter.ProtocolConverterObservedStateSnapshot) fsm.FSMInstanceSnapshot {
	return fsm.FSMInstanceSnapshot{
		ID:                id,
		CurrentState:      protocolconverter.OperationalStateActive,
		DesiredState:      protocolconverter.OperationalStateActive,
		LastObservedState: snap,
	}
}

var _ = Describe("regression: no FSM behavior change from Bridge population", func() {
	log := zap.NewNop().Sugar()

	It("isInitialized keys off read input presence, not Bridge", func() {
		dfc, err := buildProtocolConverterAsDfc(toInstance("pc-init", bidirectionalSnap()), log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.IsInitialized).To(BeTrue(), "read input present")

		writeOnly := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "questdb"},
					},
				},
			},
		}
		dfc2, err := buildProtocolConverterAsDfc(toInstance("pc-wo", writeOnly), log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc2.IsInitialized).To(BeFalse(), "read-biased; write-only stays false until ENG-5251")
	})

	It("Health reflects instance.CurrentState, not Bridge", func() {
		dfc, err := buildProtocolConverterAsDfc(toInstance("pc-health", bidirectionalSnap()), log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.Health).NotTo(BeNil())
		Expect(dfc.Health.ObservedState).To(Equal(protocolconverter.OperationalStateActive))
	})

	It("Metrics stays nil when no benthos metrics observed", func() {
		dfc, err := buildProtocolConverterAsDfc(toInstance("pc-metrics", bidirectionalSnap()), log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.Metrics).To(BeNil())
	})

	It("reconcile transient: empty write FSM state leaves writeFlowHealth nil, Bridge still populated (case 15)", func() {
		dfc, err := buildProtocolConverterAsDfc(toInstance("pc-transient", bidirectionalSnap()), log)
		Expect(err).NotTo(HaveOccurred())
		Expect(dfc.WriteFlowHealth).To(BeNil(), "buildDFCFlowHealth returns nil on empty FSM state")
		Expect(dfc.ReadFlowHealth).To(BeNil())
		Expect(dfc.Bridge).NotTo(BeNil(), "Bridge populated regardless of the transient")
		Expect(dfc.Bridge.InputType).To(Equal("http_client"))
	})
})

var _ = Describe("auditability: debug-logs when a configured side resolves to an empty protocol id", func() {
	// The generator debug-logs two divergences that otherwise leave an operator
	// asking "why does my configured bridge render as not-configured?" once
	// ENG-5249 gates on bridge.inputType/outputType: a non-empty read input whose
	// BenthosPluginID resolves to "" (compound/malformed), and a configured write
	// side with no Destination.Protocol (Code-without-Protocol). These logs are
	// the only signal for either case.

	It("logs when read input is non-empty but BenthosPluginID returns empty (compound input)", func() {
		core, logs := observer.New(zapcore.DebugLevel)
		log := zap.New(core).Sugar()

		compound := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{
								"mqtt":  map[string]any{"url": "tcp://x"},
								"opcua": map[string]any{"endpoint": "opc.tcp://y"},
							},
						},
					},
				},
			},
		}
		_, err := buildProtocolConverterAsDfc(toInstance("pc-compound", compound), log)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs.FilterMessageSnippet("BenthosPluginID").All()).
			To(HaveLen(1), "must debug-log the empty-result-for-non-empty-read-input case")
	})

	It("logs when write side is configured (HasOutput) but Destination.Protocol is empty (Code-without-Protocol)", func() {
		core, logs := observer.New(zapcore.DebugLevel)
		log := zap.New(core).Sugar()

		codeOnly := &protocolconverter.ProtocolConverterObservedStateSnapshot{
			ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
				Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
					DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]any{"input": map[string]any{"http_client": map[string]any{"url": "http://x"}}},
						},
					},
					DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
						Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Code: "self|json"},
					},
				},
			},
		}
		_, err := buildProtocolConverterAsDfc(toInstance("pc-code", codeOnly), log)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs.FilterMessageSnippet("Code-without-Protocol").All()).
			To(HaveLen(1), "must debug-log the HasOutput-true/Protocol-empty divergence")
	})
})
