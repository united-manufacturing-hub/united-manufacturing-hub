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

package actions_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

// recordedSentryEvent captures one SentryWarn/SentryError call: the event
// name and the structured fields it carried.
type recordedSentryEvent struct {
	name   string
	fields []deps.Field
}

// fieldKeys returns the keys of the event's structured fields.
func (e recordedSentryEvent) fieldKeys() []string {
	keys := make([]string, 0, len(e.fields))
	for _, f := range e.fields {
		keys = append(keys, f.Key)
	}

	return keys
}

// recordingFSMLogger is a deps.FSMLogger that records Sentry-bound events so
// specs can pin which events fire and what they carry (and, in particular,
// that no event carries the full config with its user variables).
type recordingFSMLogger struct {
	mu     sync.Mutex
	events []recordedSentryEvent
}

func (l *recordingFSMLogger) Debug(msg string, fields ...deps.Field) {}
func (l *recordingFSMLogger) Info(msg string, fields ...deps.Field)  {}

func (l *recordingFSMLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	l.record(msg, fields)
}

func (l *recordingFSMLogger) SentryError(feature deps.Feature, hierarchyPath string, err error, msg string, fields ...deps.Field) {
	l.record(msg, fields)
}

func (l *recordingFSMLogger) With(fields ...deps.Field) deps.FSMLogger { return l }

func (l *recordingFSMLogger) record(name string, fields []deps.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, recordedSentryEvent{name: name, fields: fields})
}

// eventNames returns the names of all recorded Sentry events in order.
func (l *recordingFSMLogger) eventNames() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	names := make([]string, 0, len(l.events))
	for _, e := range l.events {
		names = append(names, e.name)
	}

	return names
}

// eventsNamed returns all recorded Sentry events with the given name.
func (l *recordingFSMLogger) eventsNamed(name string) []recordedSentryEvent {
	l.mu.Lock()
	defer l.mu.Unlock()

	var matched []recordedSentryEvent

	for _, e := range l.events {
		if e.name == name {
			matched = append(matched, e)
		}
	}

	return matched
}

var _ = Describe("EditProtocolConverter", func() {
	// Variables used across tests
	var (
		action          *actions.EditProtocolConverterAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		stateMocker     *actions.StateMocker
		messages        []*models.UMHMessage
		pcName          string
		pcUUID          uuid.UUID
		mu              sync.Mutex
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		mu.Lock()
		messages = nil
		mu.Unlock()
		pcName = "wetter"
		pcUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(pcName)

		// Create initial config with a basic protocol converter (like the wetter in config.yaml)
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
				Location: map[int]string{
					0: "test-enterprise",
					1: "test-site",
					2: "test-area",
					3: "test-line",
				},
			},
			ProtocolConverter: []config.ProtocolConverterConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            pcName,
						DesiredFSMState: "active",
					},
					ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "{{ .IP }}",
									Port:   "{{ .PORT }}",
								},
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{
								"IP":   "wttr.in",
								"PORT": "80",
							},
						},
						Location: map[string]string{
							"0": "test-enterprise",
							"1": "test-site",
							"2": "test-area",
							"3": "test-line",
						},
					},
				},
			},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)

		// Setup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()

		action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)
	})

	// Cleanup after each test
	AfterEach(func() {
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid read DFC payload", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"location": map[string]interface{}{
					"0": "test-enterprise",
					"1": "test-site",
					"2": "test-area",
					"3": "test-line",
				},
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: null\n    verb: GET\n    headers: {}\n    timeout: 5s\n    retry_period: 1s\n    rate_limit: http_limiter",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  # these variables are autogenerated from your instance location\n  let enterprise = \"test-enterprise\"\n  let site = \"test-site\"\n  let area = \"test-area\"\n  let line = \"test-line\"\n  let schema = \"_historian\"\n\n  let payloadTagName = \"EUR_BTC\"\n  let value = this.bpi.EUR.rate_float\n  let tagname = \"machineState\"\n  let tagtype = \"raw\"\n\n  # the rest from here on will be autogenerated, see also documentation",
							},
						},
					},
					"rawYAML": map[string]interface{}{
						"data": "rate_limit_resources:\n  - label: http_limiter\n    local:\n      count: 1\n      interval: 1s",
					},
					"ignoreErrors": false,
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetProtocolConverterUUID()).To(Equal(pcUUID))
			Expect(action.GetDFCType()).To(Equal("read"))

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload).NotTo(BeNil())
			Expect(parsedPayload.BenthosConfig.Input).To(HaveKey("input"))
			Expect(parsedPayload.BenthosConfig.Pipeline).To(HaveKey("processors"))
			Expect(action.GetIgnoreHealthCheck()).To(BeFalse())
		})

		It("should parse valid write DFC payload", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"output": map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []interface{}{"localhost:9092"},
						},
					},
					"input_topics": "umh.v1.factory.*",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetProtocolConverterUUID()).To(Equal(pcUUID))
			Expect(action.GetDFCType()).To(Equal("write"))

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.Output).To(HaveKey("kafka"))
			Expect(writeCfg.InputTopics).To(Equal("umh.v1.factory.*"))
		})

		It("should return error for missing UUID", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input: something",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field UUID"))
		})

		It("should return error for invalid payload format", func() {
			// Invalid payload that cannot be parsed as ProtocolConverter
			payload := "invalid string payload"

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse protocol converter payload"))
		})

		It("should default DFC type to empty when no DFCs provided", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"uuid": pcUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(action.GetDFCType()).To(Equal("empty"))
		})

		It("should parse both read and write DFCs as DFCTypeBoth", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: root = this",
							},
						},
					},
				},
				"writeDFC": map[string]interface{}{
					"output": map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					"input_topics": "umh.v1.factory.*",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetDFCType()).To(Equal("both"))
		})

		It("should parse write DFC with umh_topics", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"output": map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					"input_topics": "umh.v1.factory.*\numh.v1.plant.*",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetDFCType()).To(Equal("write"))

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.InputTopics).To(Equal("umh.v1.factory.*\numh.v1.plant.*"))
		})

		It("should parse write DFC with golang template vars in input_topics", func() {
			// Regression test: template vars like {{ .IP }}, {{ .location_path }} must not
			// cause a parse error — rendering happens later in BuildRuntimeConfig with full scope.
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"connection": map[string]interface{}{
					"ip":   "192.168.1.1",
					"port": 502,
				},
				"writeDFC": map[string]interface{}{
					"output": map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					"input_topics": "umh.v1.{{ .location_path }}.{{ .IP }}.*",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.InputTopics).To(Equal("umh.v1.{{ .location_path }}.{{ .IP }}.*"))
		})

		It("should preserve explicit 'stopped' state on read DFC", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: root = this",
							},
						},
					},
					"state": "stopped",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(action.GetDFCType()).To(Equal("read"))
		})
	})

	Describe("Validate", func() {
		It("should validate valid read DFC configuration", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for invalid UUID", func() {
			// Use action with empty UUID
			action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing or invalid protocol converter UUID"))
		})

		It("should validate write DFC without explicit inputs (auto-generated)", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"output": map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					"input_topics": "umh.v1.factory.*",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass validation for write DFC state-only change without umh_topics", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"state": "stopped",
					// no outputs, no umh_topics — state-only change
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when write DFC has no umh_topics", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"output": map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					// no umh_topics
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("input_topics"))
		})

		It("should return error for invalid DFC configuration", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "", // Empty data
						"type": "", // Empty type
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{},
					},
				},
			}

			// Validation happens during Parse (build step validates the DFC)
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid read DFC configuration"))
		})
	})

	Describe("Execute", func() {
		It("should successfully add read DFC to empty protocol converter", func() {
			// Parse the payload
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"location": map[string]interface{}{
					"0": "test-enterprise",
					"1": "test-site",
					"2": "test-area",
					"3": "test-line",
				},
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: null\n    verb: GET\n    headers: {}\n    timeout: 5s\n    retry_period: 1s\n    rate_limit: http_limiter",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  # these variables are autogenerated from your instance location\n  let enterprise = \"test-enterprise\"\n  let site = \"test-site\"\n  let area = \"test-area\"\n  let line = \"test-line\"\n  let schema = \"_historian\"\n\n  let payloadTagName = \"EUR_BTC\"\n  let value = this.bpi.EUR.rate_float\n  let tagname = \"machineState\"\n  let tagtype = \"raw\"\n\n  # the rest from here on will be autogenerated, see also documentation",
							},
						},
					},
					"rawYAML": map[string]interface{}{
						"data": "rate_limit_resources:\n  - label: http_limiter\n    local:\n      count: 1\n      interval: 1s",
					},
					"ignoreErrors": false,
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			_, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify that the config was actually updated
			ctx := context.Background()
			updatedConfig, err := mockConfig.GetConfig(ctx, 0)
			Expect(err).NotTo(HaveOccurred())

			// Find the updated protocol converter
			var updatedPC *config.ProtocolConverterConfig
			for i, pc := range updatedConfig.ProtocolConverter {
				if pc.Name == pcName {
					updatedPC = &updatedConfig.ProtocolConverter[i]

					break
				}
			}
			Expect(updatedPC).NotTo(BeNil(), "Protocol converter should exist in updated config")

			// Verify the read DFC was added to the protocol converter configuration
			readDFCConfig := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig
			Expect(readDFCConfig.BenthosConfig.Input).NotTo(BeEmpty())
			Expect(readDFCConfig.BenthosConfig.Input["input"]).To(HaveKey("http_client"))
			Expect(readDFCConfig.BenthosConfig.Pipeline).NotTo(BeEmpty())

			// Verify write DFC is still empty
			writeDFCConfig := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig
			Expect(writeDFCConfig.HasOutput()).To(BeFalse())

			// verify that the templateRef is set
			Expect(updatedPC.ProtocolConverterServiceConfig.TemplateRef).To(Equal(pcName))
		})

		It("should store InputTopics in typed write config when adding write DFC", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"output": map[string]interface{}{
						"stdout": map[string]interface{}{},
					},
					"input_topics": "umh.v1.factory.line-1.*\numh.v1.factory.line-2.*",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			_, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			stateMocker.Stop()

			// Verify InputTopics stored in typed write config
			ctx := context.Background()
			updatedConfig, err := mockConfig.GetConfig(ctx, 0)
			Expect(err).NotTo(HaveOccurred())

			var updatedPC *config.ProtocolConverterConfig
			for i, pc := range updatedConfig.ProtocolConverter {
				if pc.Name == pcName {
					updatedPC = &updatedConfig.ProtocolConverter[i]

					break
				}
			}
			Expect(updatedPC).NotTo(BeNil())
			Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.InputTopics).To(Equal("umh.v1.factory.line-1.*\numh.v1.factory.line-2.*"))
		})

		Context("when protocol converter already has both DFCs configured", func() {
			// overwrite the BeforeEach config with one that has both DFCs populated
			BeforeEach(func() {
				existingReadInput := map[string]interface{}{
					"http_client": map[string]interface{}{
						"url": "http://original.example.com",
					},
				}
				existingWriteOutput := map[string]interface{}{
					"stdout": map[string]interface{}{},
				}

				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: existingReadInput,
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Output:      existingWriteOutput,
					InputTopics: "umh.v1.factory.*",
				}

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should not change read DFC when only write DFC is edited", Label("write-dfc", "isolation"), func() {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"writeDFC": map[string]interface{}{
						"output": map[string]interface{}{
							"kafka": map[string]interface{}{
								"addresses": []interface{}{"localhost:9092"},
							},
						},
						"input_topics": "umh.v1.plant.*",
					},
				}

				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				err = action.Validate()
				Expect(err).NotTo(HaveOccurred())

				err = stateMocker.Start()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				Expect(err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())

				var updatedPC *config.ProtocolConverterConfig
				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						updatedPC = &updatedConfig.ProtocolConverter[i]

						break
					}
				}
				Expect(updatedPC).NotTo(BeNil())

				// Write DFC output must have changed — new format stores directly without wrapper
				writeOutput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Output
				Expect(writeOutput).To(HaveKey("kafka"))

				// Read DFC input must be untouched — initial config set directly without wrapper
				readInput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Input
				Expect(readInput).To(HaveKey("http_client"))
				Expect(readInput["http_client"]).To(HaveKeyWithValue("url", "http://original.example.com"))
			})

			It("should not change write DFC when only read DFC is edited", Label("write-dfc", "isolation"), func() {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: http://updated.example.com",
							"type": "http_client",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "bloblang",
									"data": "bloblang: |-\n  root = content()",
								},
							},
						},
					},
				}

				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				err = action.Validate()
				Expect(err).NotTo(HaveOccurred())

				err = stateMocker.Start()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				Expect(err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())

				var updatedPC *config.ProtocolConverterConfig
				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						updatedPC = &updatedConfig.ProtocolConverter[i]

						break
					}
				}
				Expect(updatedPC).NotTo(BeNil())

				// Read DFC input must have changed — parsed YAML wraps in outer "input" key
				readInput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Input
				Expect(readInput).To(HaveKey("input"))
				Expect(readInput["input"]).To(HaveKey("http_client"))
				Expect(readInput["input"].(map[string]interface{})["http_client"]).To(HaveKeyWithValue("url", "http://updated.example.com"))

				// Write DFC output must be untouched — initial config set directly without wrapper
				writeOutput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Output
				Expect(writeOutput).To(HaveKey("stdout"))

				// InputTopics must be untouched
				Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.InputTopics).To(Equal("umh.v1.factory.*"))
			})
		})

		Context("when starting and stopping individual DFCs", func() {
			// Minimal valid read DFC payload reused across several tests below.
			// The pipeline has one bloblang processor so read-DFC validation passes.
			minimalReadDFCPayload := func(state string) map[string]interface{} {
				return map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: root = content()",
							},
						},
					},
					"state": state,
				}
			}

			// Minimal write DFC payload: no inputs, pipeline, or input_topics needed for
			// a state-only change. InputTopics is only required when output config is present.
			minimalWriteDFCPayload := func(state string) map[string]interface{} {
				return map[string]interface{}{
					"state": state,
				}
			}

			// getUpdatedPC is a helper that runs Parse+Validate+Execute and returns the
			// saved protocol converter config, asserting no error along the way.
			runEdit := func(payloadMap map[string]interface{}) *config.ProtocolConverterConfig {
				err := action.Parse(payloadMap)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				err = action.Validate()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				err = stateMocker.Start()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						return &updatedConfig.ProtocolConverter[i]
					}
				}
				Fail("protocol converter not found after execute")

				return nil
			}

			BeforeEach(func() {
				// Both DFCs are present and explicitly set to "active" so that
				// dfcPayloadDiffers can use the state-diff short-circuit path.
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]interface{}{"http_client": map[string]interface{}{"url": "http://example.com"}},
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Output:      map[string]interface{}{"stdout": map[string]interface{}{}},
					InputTopics: "umh.v1.factory.*",
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "active"

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should stop read DFC only", Label("disable-bridges"), func() {
				pc := runEdit(map[string]interface{}{
					"name":    pcName,
					"uuid":    pcUUID.String(),
					"readDFC": minimalReadDFCPayload("stopped"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("stopped"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
			})

			It("should start read DFC from stopped", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "stopped"

				pc := runEdit(map[string]interface{}{
					"name":    pcName,
					"uuid":    pcUUID.String(),
					"readDFC": minimalReadDFCPayload("active"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
			})

			It("should stop write DFC only", Label("disable-bridges"), func() {
				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"writeDFC": minimalWriteDFCPayload("stopped"),
				})

				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("stopped"))
				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
			})

			It("should start write DFC from stopped", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "stopped"

				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"writeDFC": minimalWriteDFCPayload("active"),
				})

				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
			})

			It("should stop both DFCs simultaneously", Label("disable-bridges"), func() {
				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"readDFC":  minimalReadDFCPayload("stopped"),
					"writeDFC": minimalWriteDFCPayload("stopped"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("stopped"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("stopped"))
			})

			It("should start both DFCs simultaneously", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "stopped"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "stopped"

				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"readDFC":  minimalReadDFCPayload("active"),
					"writeDFC": minimalWriteDFCPayload("active"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
			})
		})

		It("should handle protocol converter not found error", func() {
			// Use a non-existent UUID
			nonExistentUUID := uuid.New()
			payload := map[string]interface{}{
				"name": "non-existent",
				"uuid": nonExistentUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: http://example.com",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail with protocol converter not found
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("protocol converter with UUID"))
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(metadata).To(BeNil())
		})

		It("should handle AtomicEditProtocolConverter failure", func() {
			// Set up mock to fail on AtomicEditProtocolConverter
			mockConfig.WithAtomicEditProtocolConverterError(errors.New("mock edit protocol converter failure"))

			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: http://example.com",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update protocol converter: mock edit protocol converter failure"))
			Expect(metadata).To(BeNil())
		})

		Context("deriveDFCType smart diff", func() {
			// deployReadDFC executes a first edit that stores the given read DFC payload
			// into the mock config so the next Parse can detect "no change".
			deployReadDFC := func(readDFC map[string]interface{}) {
				firstPayload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"readDFC": readDFC,
				}
				firstAction := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				err := firstAction.Parse(firstPayload)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				err = firstAction.Validate()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				_, _, err = firstAction.Execute()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			// baseReadDFC is a minimal valid read DFC configuration.
			baseReadDFC := map[string]interface{}{
				"inputs": map[string]interface{}{
					"data": "input:\n  http_client:\n    url: 'http://example.com'",
					"type": "http_client",
				},
				"pipeline": map[string]interface{}{
					"processors": map[string]interface{}{
						"0": map[string]interface{}{
							"type": "bloblang",
							"data": "bloblang: root = content()",
						},
					},
				},
				"state": "active",
			}

			It("should return DFCTypeEmpty when read DFC config is identical to deployed", func() {
				deployReadDFC(baseReadDFC)

				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"readDFC": baseReadDFC,
				}
				err := action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action2.GetDFCType()).To(Equal("empty"))
			})

			It("should return DFCTypeRead when per-DFC state changes from active to stopped", func() {
				deployReadDFC(baseReadDFC)

				stoppedDFC := make(map[string]interface{})
				for k, v := range baseReadDFC {
					stoppedDFC[k] = v
				}
				stoppedDFC["state"] = "stopped"

				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"readDFC": stoppedDFC,
				}
				err := action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action2.GetDFCType()).To(Equal("read"))
			})

			It("should return DFCTypeWrite when InputTopics differ from deployed", func() {
				// First, deploy a write DFC with a known topic set.
				firstWritePayload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"writeDFC": map[string]interface{}{
						"output": map[string]interface{}{
							"stdout": map[string]interface{}{},
						},
						"input_topics": "umh.v1.factory.*",
					},
				}
				firstAction := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				err := firstAction.Parse(firstWritePayload)
				Expect(err).NotTo(HaveOccurred())
				err = firstAction.Validate()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = firstAction.Execute()
				Expect(err).NotTo(HaveOccurred())

				// Now parse the same config but with different topics.
				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"writeDFC": map[string]interface{}{
						"output": map[string]interface{}{
							"stdout": map[string]interface{}{},
						},
						"input_topics": "umh.v1.plant.*", // different topic
					},
				}
				err = action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action2.GetDFCType()).To(Equal("write"))
			})

			It("should return DFCTypeBoth when a custom template variable changes", Label("write-dfc"), func() {
				// First edit: deploy read + write with a custom template variable.
				firstPayload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "10.0.0.1",
						"port": 502,
					},
					"templateInfo": map[string]interface{}{
						"variables": []interface{}{
							map[string]interface{}{"label": "baudRate", "value": "9600"},
						},
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://{{ .baudRate }}'",
							"type": "http_client",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "bloblang",
									"data": "bloblang: root = content()",
								},
							},
						},
						"state": "active",
					},
					"writeDFC": map[string]interface{}{
						"output":       map[string]interface{}{"stdout": map[string]interface{}{}},
						"input_topics": "umh.v1.factory.*",
					},
				}
				firstAction := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				err := firstAction.Parse(firstPayload)
				Expect(err).NotTo(HaveOccurred())
				err = firstAction.Validate()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = firstAction.Execute()
				Expect(err).NotTo(HaveOccurred())

				// Second edit: change only baudRate — DFC config text is identical.
				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "10.0.0.1",
						"port": 502,
					},
					"templateInfo": map[string]interface{}{
						"variables": []interface{}{
							map[string]interface{}{"label": "baudRate", "value": "19200"}, // changed
						},
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://{{ .baudRate }}'",
							"type": "http_client",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "bloblang",
									"data": "bloblang: root = content()",
								},
							},
						},
						"state": "active",
					},
					"writeDFC": map[string]interface{}{
						"output":       map[string]interface{}{"stdout": map[string]interface{}{}},
						"input_topics": "umh.v1.factory.*",
					},
				}
				err = action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				// baudRate changed → both DFCs need redeploy even though DFC config text is identical.
				Expect(action2.GetDFCType()).To(Equal("both"))
			})
		})

		Context("DFCTypeEmpty edit preserves per-DFC stopped states", func() {
			BeforeEach(func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "stopped"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "stopped"

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should preserve ReadDFCDesiredState and WriteDFCDesiredState when no DFC payload is sent", Label("disable-bridges"), func() {
				// Connection-only edit: no readDFC or writeDFC in payload.
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
				}

				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action.GetDFCType()).To(Equal("empty"))

				err = action.Validate()
				Expect(err).NotTo(HaveOccurred())

				err = stateMocker.Start()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				Expect(err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())

				var updatedPC *config.ProtocolConverterConfig
				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						updatedPC = &updatedConfig.ProtocolConverter[i]

						break
					}
				}
				Expect(updatedPC).NotTo(BeNil())
				Expect(updatedPC.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("stopped"))
				Expect(updatedPC.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("stopped"))
			})
		})

		Context("DFCTypeBoth execute path", func() {
			BeforeEach(func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]interface{}{"http_client": map[string]interface{}{"url": "http://old.example.com"}},
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Output:      map[string]interface{}{"stdout": map[string]interface{}{}},
					InputTopics: "umh.v1.factory.*",
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "active"

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should write both DFC configs when editing both simultaneously", Label("write-dfc"), func() {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: http://new.example.com",
							"type": "http_client",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "bloblang",
									"data": "bloblang: root = content()",
								},
							},
						},
					},
					"writeDFC": map[string]interface{}{
						"output": map[string]interface{}{
							"kafka": map[string]interface{}{
								"addresses": []interface{}{"localhost:9092"},
							},
						},
						"input_topics": "umh.v1.plant.*",
					},
				}

				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action.GetDFCType()).To(Equal("both"))

				err = action.Validate()
				Expect(err).NotTo(HaveOccurred())

				err = stateMocker.Start()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				Expect(err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())

				var updatedPC *config.ProtocolConverterConfig
				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						updatedPC = &updatedConfig.ProtocolConverter[i]

						break
					}
				}
				Expect(updatedPC).NotTo(BeNil())

				// Read DFC: parsed YAML wraps in outer "input" key
				readInput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Input
				Expect(readInput).To(HaveKey("input"))
				Expect(readInput["input"]).To(HaveKey("http_client"))
				Expect(readInput["input"].(map[string]interface{})["http_client"]).To(HaveKeyWithValue("url", "http://new.example.com"))

				// Write DFC: new format stores directly without wrapper
				writeOutput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Output
				Expect(writeOutput).To(HaveKey("kafka"))

				// InputTopics updated
				Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.InputTopics).To(Equal("umh.v1.plant.*"))
			})
		})

		Context("compareSingleDFCConfig via awaitRollout", func() {
			// These tests exercise compareSingleDFCConfig by passing a real
			// systemSnapshotManager populated with a PC snapshot, so awaitRollout
			// actually runs and calls compareSingleDFCConfig on its first tick.

			It("should complete when read DFC has reached the stopped state", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]interface{}{"http_client": map[string]interface{}{"url": "http://example.com"}},
					},
				}

				snapshotMgr := fsm.NewSnapshotManager()

				// Build a PC snapshot: overall state "active", read DFC FSM state "stopped".
				observedState := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentReadFSMState: protocolconverter.OperationalStateStopped,
					},
				}
				instance := &fsm.FSMInstanceSnapshot{
					ID:                pcName,
					CurrentState:      protocolconverter.OperationalStateActive,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: observedState,
				}
				snapshotMgr.UpdateSnapshot(&fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
							Instances: map[string]*fsm.FSMInstanceSnapshot{pcName: instance},
						},
					},
				})

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, snapshotMgr)

				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://example.com'",
							"type": "http_client",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "bloblang",
									"data": "bloblang: root = content()",
								},
							},
						},
						"state": "stopped",
					},
				}

				err := localAction.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				err = localAction.Validate()
				Expect(err).NotTo(HaveOccurred())

				type execResult struct {
					err error
				}
				ch := make(chan execResult, 1)
				go func() {
					_, _, err := localAction.Execute()
					ch <- execResult{err}
				}()

				Eventually(ch, "5s").Should(Receive(And(
					WithTransform(func(r execResult) error { return r.err }, BeNil()),
				)))
			})

			It("should complete when write DFC has reached the stopped state", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Output:      map[string]interface{}{"stdout": map[string]interface{}{}},
					InputTopics: "umh.v1.factory.*",
				}

				snapshotMgr := fsm.NewSnapshotManager()

				observedState := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentWriteFSMState: protocolconverter.OperationalStateStopped,
					},
				}
				instance := &fsm.FSMInstanceSnapshot{
					ID:                pcName,
					CurrentState:      protocolconverter.OperationalStateActive,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: observedState,
				}
				snapshotMgr.UpdateSnapshot(&fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
							Instances: map[string]*fsm.FSMInstanceSnapshot{pcName: instance},
						},
					},
				})

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, snapshotMgr)

				payload := map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"writeDFC": map[string]interface{}{"state": "stopped"},
				}

				err := localAction.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				err = localAction.Validate()
				Expect(err).NotTo(HaveOccurred())

				type execResult struct {
					err error
				}
				ch := make(chan execResult, 1)
				go func() {
					_, _, err := localAction.Execute()
					ch <- execResult{err}
				}()

				Eventually(ch, "5s").Should(Receive(And(
					WithTransform(func(r execResult) error { return r.err }, BeNil()),
				)))
			})
		})

		Context("fail-fast on persistent render failures", func() {
			// ENG-5103: a deterministic render failure used to retry every
			// tick for the full 30s timeout. The await loop must abort after
			// 3 consecutive identical render failures instead — but never on
			// the first failure (transient snapshot staleness), never while
			// the error keeps changing, and any tick whose comparison runs
			// without a render failure breaks the streak. Ticks that skip
			// the comparison entirely (manager, instance or state info
			// missing from the snapshot) leave the streak untouched.

			// tick is the awaitRollout poll interval, injected via
			// SetTickInterval. Short enough to keep these specs off the 1s
			// production ticker, long enough that the reply collector has a
			// comfortable window to mutate the snapshot before the next poll.
			tick := 100 * time.Millisecond

			type capturedReply struct {
				state     string
				message   string
				errorCode string
			}

			var (
				snapshotMgr   *fsm.SnapshotManager
				localOutbound chan *models.UMHMessage
				replyMu       sync.Mutex
				replies       []capturedReply
				sentryRec     *recordingFSMLogger
			)

			BeforeEach(func() {
				snapshotMgr = fsm.NewSnapshotManager()
				sentryRec = &recordingFSMLogger{}
				// Dedicated outbound channel: in the no-fail-fast case the
				// Execute goroutine keeps sending progress replies past this
				// spec's lifetime, and the suite-level channel is closed in
				// AfterEach. This one is collected forever and never closed.
				localOutbound = make(chan *models.UMHMessage, 100)
				replyMu.Lock()
				replies = nil
				replyMu.Unlock()
			})

			// observedStateWithInject builds a PC observed-state snapshot.
			// A multi-line inject value (such as "x\nbroken: [") expands to
			// a column-0 line inside a block scalar: yaml.Marshal and
			// template execution succeed, unmarshalling the rendered output
			// fails — deterministically, with an error message embedding the
			// inject value. A single-line inject value renders fine; the
			// comparison then fails without a render error because the
			// observed Benthos config does not match the rendered desired
			// config.
			observedStateWithInject := func(inject string) *protocolconverter.ProtocolConverterObservedStateSnapshot {
				input := map[string]interface{}{"http_client": map[string]interface{}{}}

				return &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "localhost",
									Port:   "102",
								},
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{"inject": inject},
						},
						Location: map[string]string{"0": "test-enterprise"},
					},
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentReadFSMState: protocolconverter.OperationalStateIdle,
						DataflowComponentReadObservedState: dfcfsm.DataflowComponentObservedState{
							ServiceInfo: dfcsvc.ServiceInfo{
								BenthosObservedState: benthosfsm.BenthosObservedState{
									ObservedBenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
										Input: input,
									},
								},
							},
						},
					},
				}
			}

			updateSnapshot := func(observedState *protocolconverter.ProtocolConverterObservedStateSnapshot) {
				instance := &fsm.FSMInstanceSnapshot{
					ID:                pcName,
					CurrentState:      protocolconverter.OperationalStateIdle,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: observedState,
				}
				snapshotMgr.UpdateSnapshot(&fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
							Instances: map[string]*fsm.FSMInstanceSnapshot{pcName: instance},
						},
					},
				})
			}

			getReplies := func() []capturedReply {
				replyMu.Lock()
				defer replyMu.Unlock()

				return append([]capturedReply(nil), replies...)
			}

			countNotYetApplied := func() int {
				count := 0
				for _, r := range getReplies() {
					if strings.Contains(r.message, "DFC config not yet applied") {
						count++
					}
				}

				return count
			}

			// startCollector decodes every outbound reply into replies and
			// invokes onProgress with the running count of "DFC config not
			// yet applied" ticks, letting tests mutate the snapshot between
			// poll ticks (each progress reply follows one comparison, and
			// the next comparison is a full tick away).
			startCollector := func(onProgress func(notYetAppliedCount int)) {
				go func() {
					notYetApplied := 0
					for msg := range localOutbound {
						dec, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
						if err != nil {
							continue
						}
						payload, ok := dec.Payload.(map[string]interface{})
						if !ok {
							continue
						}
						var r capturedReply
						r.state, _ = payload["actionReplyState"].(string)
						r.message, _ = payload["actionReplyPayload"].(string)
						if v2, ok := payload["actionReplyPayloadV2"].(map[string]interface{}); ok {
							r.errorCode, _ = v2["errorCode"].(string)
						}
						replyMu.Lock()
						replies = append(replies, r)
						replyMu.Unlock()
						if onProgress != nil && strings.Contains(r.message, "DFC config not yet applied") {
							notYetApplied++
							onProgress(notYetApplied)
						}
					}
				}()
			}

			brokenReadDFCPayload := func() map[string]interface{} {
				return map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							// multi-line value marshals as a block scalar;
							// {{ .inject }} expands to a column-0 line that
							// terminates it and breaks the YAML
							"data": "input:\n  http_client:\n    url: |-\n      line1\n      {{ .inject }}",
							"type": "http_client",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "bloblang",
									"data": "bloblang: root = content()",
								},
							},
						},
						"state": "active",
					},
				}
			}

			// brokenWriteDFCPayload embeds a multi-line block-scalar
			// value in the output map. yaml.Marshal turns it into a
			// block literal; {{ .inject }} expands to a column-0 line
			// that terminates the scalar and breaks YAML unmarshal.
			brokenWriteDFCPayload := func() map[string]interface{} {
				return map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"writeDFC": map[string]interface{}{
						"output": map[string]interface{}{
							"http_client": map[string]interface{}{
								"url": "line1\n{{ .inject }}",
							},
						},
						"input_topics": "umh.v1.factory.*",
					},
				}
			}

			// newVariablePayload is a VALID edit that introduces a template variable
			// the observed spec does not have yet.
			newVariablePayload := func() map[string]interface{} {
				return map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"templateInfo": map[string]interface{}{
						"variables": []interface{}{
							map[string]interface{}{"label": "newvar", "value": "example.com"},
						},
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							// References BOTH the edit-carried variable (newvar)
							// and an observed-only one (inject): the render must
							// merge the edit's variables OVER the observed ones,
							// not replace them.
							"data": "input:\n  http_client:\n    url: 'http://{{ .newvar }}/{{ .inject }}'",
							"type": "http_client",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "bloblang",
									"data": "bloblang: root = content()",
								},
							},
						},
						"state": "active",
					},
				}
			}

			runEdit := func(within time.Duration) (time.Duration, error) {
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				localAction.SetTickInterval(tick)
				localAction.SetFSMLogger(sentryRec)

				Expect(localAction.Parse(brokenReadDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				type execResult struct {
					err error
				}
				ch := make(chan execResult, 1)
				start := time.Now()
				go func() {
					_, _, err := localAction.Execute()
					ch <- execResult{err}
				}()

				var result execResult
				Eventually(ch, within).Should(Receive(&result))

				return time.Since(start), result.err
			}

			// finalFailureReply waits for the terminal failure reply (sent
			// just before Execute returns, so the collector may still be
			// catching up) and returns it.
			finalFailureReply := func() capturedReply {
				var final capturedReply
				Eventually(func() bool {
					for _, r := range getReplies() {
						if r.state == string(models.ActionFinishedWithFailure) {
							final = r

							return true
						}
					}

					return false
				}, "2s").Should(BeTrue(), "expected a final ActionFinishedWithFailure reply")

				return final
			}

			It("aborts the rollout after 3 consecutive identical render failures, rolls back, and reports the error code", func() {
				updateSnapshot(observedStateWithInject("x\nbroken: ["))
				startCollector(nil)

				// Without fail-fast this takes the full timeout; half of it
				// splits the two outcomes safely.
				elapsed, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout / 2)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("render"))
				Expect(execErr.Error()).To(ContainSubstring("was rolled back"))
				// Never on the first failure: three identical failures need
				// at least two full ticks after the first.
				Expect(elapsed).To(BeNumerically(">=", 2*tick))

				// The rollback must actually write the pre-edit config back:
				// call 1 persists the edit, call 2 is the rollback.
				Expect(mockConfig.AtomicEditProtocolConverterCallCount).To(Equal(2))
				Expect(mockConfig.AtomicEditProtocolConverterLastUUID).To(Equal(pcUUID))
				Expect(mockConfig.AtomicEditProtocolConverterLastConfig.Name).To(Equal(pcName))
				// The pre-edit config had no read DFC; the broken edit added one.
				Expect(mockConfig.AtomicEditProtocolConverterLastConfig.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig).
					To(Equal(dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}))

				// The frontend uses the error code to decide retryability:
				// the config was safely rolled back, so this is the
				// fix-your-config case, not the retry case.
				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrConfigFileInvalid))
				Expect(final.message).To(ContainSubstring("was rolled back"))

				// Pin the threshold: 3 identical failures abort on the third
				// tick, so exactly the first two ticks produced an ordinary
				// progress reply before the rollback announcement.
				Expect(countNotYetApplied()).To(Equal(2),
					"fail-fast threshold moved: expected maxIdenticalRenderFails-1 progress ticks before rollback")

				rollingBack := 0
				for _, r := range getReplies() {
					if strings.Contains(r.message, "persistent render failure detected. Rolling back...") {
						rollingBack++
					}
				}
				Expect(rollingBack).To(Equal(1))

				// Pin what reaches Sentry: the dedicated render-failure event
				// carries the bridge name, UUID and render error (which includes
				// the rendered-output snippet; that exposure is an accepted
				// decision), and the generic rollout_failed event, which attaches
				// the full old/new config, must not fire on top of it.
				events := sentryRec.eventNames()
				Expect(events).To(ContainElement("edit_protocol_converter_render_failure_rolled_back"))
				Expect(events).NotTo(ContainElement("edit_protocol_converter_rollout_failed"))
				rolledBack := sentryRec.eventsNamed("edit_protocol_converter_render_failure_rolled_back")
				Expect(rolledBack).To(HaveLen(1))
				Expect(rolledBack[0].fieldKeys()).To(ConsistOf(
					"protocolConverter", "protocolConverterUUID", "renderErr"))
			})

			It("keeps waiting while the render error keeps changing, aborting only after it stabilises", func() {
				updateSnapshot(observedStateWithInject("x\nbroken0: ["))
				startCollector(nil)

				// Drive snapshot swaps reactively: after each "DFC config
				// not yet applied" reply confirms a tick ran with variant n,
				// advance to variant n+1. This avoids the onProgress-callback
				// race where the swap may land AFTER the next tick started
				// reading the old snapshot under a loaded CI runner.
				// After variant 5, the snapshot is held constant so the
				// streak can finally build to 3 and trigger the abort.
				go func() {
					defer GinkgoRecover()
					for n := 1; n <= 5; n++ {
						// Wait until n "DFC config not yet applied" replies
						// have arrived, confirming n ticks ran.
						Eventually(func() int {
							return countNotYetApplied()
						}, "5s", "10ms").Should(BeNumerically(">=", n))
						updateSnapshot(observedStateWithInject(fmt.Sprintf("x\nbroken%d: [", n)))
					}
				}()

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout - 5*time.Second)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("was rolled back"))

				// Wait for the terminal reply before counting: the collector
				// may still be draining the channel when runEdit returns.
				finalFailureReply()

				// Each of the 5 rotating variants produced at least one
				// "not yet applied" reply (streak never above 1 during that
				// phase). Two more ticks with the held variant raise the
				// streak to 2 and then 3. Total: at least 7.
				Expect(countNotYetApplied()).To(BeNumerically(">=", 7))

				// The abort must not have fired during the rotating phase;
				// only one rollback announcement should appear in total.
				rollingBack := 0
				for _, r := range getReplies() {
					if strings.Contains(r.message, "persistent render failure detected. Rolling back...") {
						rollingBack++
					}
				}
				Expect(rollingBack).To(Equal(1))
			})

			It("resets the consecutive counter when a tick fails without a render error", func() {
				brokenInject := "x\nbroken: ["
				updateSnapshot(observedStateWithInject(brokenInject))
				startCollector(nil)

				// Drive snapshot swaps reactively to avoid the onProgress
				// callback race. Phase 1: tick 1 sees the broken inject
				// (streak=1). Phase 2: swap to benign; one tick runs with
				// benign (render succeeds, streak resets to 0). Phase 3:
				// swap back to broken; streak builds from 0, needing 3
				// consecutive identical failures before the abort fires.
				go func() {
					defer GinkgoRecover()
					// Wait for the first "DFC config not yet applied" reply
					// confirming tick 1 ran with the broken inject (streak=1),
					// then switch to benign.
					Eventually(func() int {
						return countNotYetApplied()
					}, "5s", "10ms").Should(BeNumerically(">=", 1))
					updateSnapshot(observedStateWithInject("benign"))

					// Wait for a second "DFC config not yet applied" reply
					// confirming the benign tick ran (streak reset to 0),
					// then restore the broken inject for phase 3.
					Eventually(func() int {
						return countNotYetApplied()
					}, "5s", "10ms").Should(BeNumerically(">=", 2))
					updateSnapshot(observedStateWithInject(brokenInject))
				}()

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout - 5*time.Second)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("was rolled back"))

				// Wait for the terminal reply before counting: the collector
				// may still be draining the channel when runEdit returns.
				finalFailureReply()

				// Phase 1 (broken): 1 "not yet applied" reply (streak=1).
				// Phase 2 (benign): 1 "not yet applied" reply (streak reset
				// to 0 because render succeeds).
				// Phase 3 (broken again): 2 "not yet applied" replies
				// (streak=1 and streak=2) before the abort fires on the
				// third identical failure. The abort tick sends "Rolling
				// back..." rather than "not yet applied", so the abort
				// tick itself does not count. Total: at least 4.
				Expect(countNotYetApplied()).To(BeNumerically(">=", 4))

				// Exactly one rollback must appear, only after the streak
				// rebuilt from zero in phase 3.
				rollingBack := 0
				for _, r := range getReplies() {
					if strings.Contains(r.message, "persistent render failure detected. Rolling back...") {
						rollingBack++
					}
				}
				Expect(rollingBack).To(Equal(1))
			})

			It("returns the retryable rollback-failed code when the rollback itself fails", func() {
				updateSnapshot(observedStateWithInject("x\nbroken: ["))
				startCollector(nil)

				// Call 1 is the initial persist of the edit; call 2 is the
				// rollback — fail only that one.
				mockConfig.WithAtomicEditProtocolConverterError(errors.New("simulated config write failure")).
					WithAtomicEditProtocolConverterFailOnCall(2)

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout / 2)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("could not be rolled back"))
				Expect(execErr.Error()).To(ContainSubstring("simulated config write failure"))
				Expect(mockConfig.AtomicEditProtocolConverterCallCount).To(Equal(2))

				// The system is left running the broken config — the one
				// state needing manual intervention — so the frontend must
				// not present this as "fix the config and retry"
				// (ErrConfigFileInvalid).
				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrRetryRollbackTimeout))

				// Pin what reaches Sentry: the dedicated render-failure event
				// carries the bridge name, UUID and render error (which includes
				// the rendered-output snippet; that exposure is an accepted
				// decision), and the generic rollout_failed event, which attaches
				// the full old/new config, must not fire on top of it.
				events := sentryRec.eventNames()
				Expect(events).To(ContainElement("edit_protocol_converter_render_failure_rollback_failed"))
				Expect(events).NotTo(ContainElement("edit_protocol_converter_rollout_failed"))
				failed := sentryRec.eventsNamed("edit_protocol_converter_render_failure_rollback_failed")
				Expect(failed).To(HaveLen(1))
				Expect(failed[0].fieldKeys()).To(ConsistOf(
					"protocolConverter", "protocolConverterUUID", "renderErr"))
			})

			It("keeps the render error sticky for the timeout message and clears it on the next successful render", func() {
				// The awaitRollout timeout message names lastRenderErr as the
				// root cause; this pins its lifecycle on the compare path
				// directly, without wall-clock ticks: a failing render sets
				// it, a comparison that skips the render leaves it sticky,
				// and the next successful render clears it.
				updateSnapshot(observedStateWithInject("benign"))

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(brokenReadDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateWithInject("x\nbroken: ["))
				Expect(matched).To(BeFalse())
				Expect(renderErr).To(HaveOccurred())
				Expect(localAction.LastRenderErr()).To(Equal(renderErr))

				// A comparison that never reaches a render (no snapshot for
				// the instance yet, e.g. while Benthos restarts) keeps the
				// captured cause.
				matched, renderErr = localAction.CompareProtocolConverterDFCConfig(nil)
				Expect(matched).To(BeFalse())
				Expect(renderErr).NotTo(HaveOccurred())
				Expect(localAction.LastRenderErr()).To(HaveOccurred())

				// A benign inject renders fine (the comparison itself still
				// fails because the observed config lags the edit), so the
				// stale root cause must be gone.
				matched, renderErr = localAction.CompareProtocolConverterDFCConfig(observedStateWithInject("benign"))
				Expect(matched).To(BeFalse())
				Expect(renderErr).NotTo(HaveOccurred())
				Expect(localAction.LastRenderErr()).NotTo(HaveOccurred())
			})

			It("skips the render when the observed read Benthos config is absent", func() {
				// While Benthos is stopped or restarting the observed Input map
				// is nil. Such a tick must skip the render entirely: rendering
				// against the not-yet-populated observed spec fails for reasons
				// unrelated to the edit, so it must contribute neither to the
				// fail-fast streak nor to lastRenderErr. Regression context:
				// the absence guard used to compare an interface{} sentinel
				// against nil, and a nil map stored in an interface is a typed
				// nil that compares non-nil, so the guard never fired and the
				// render ran anyway.
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(brokenReadDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				// Observed state with an empty spec and nil observed Input,
				// normal while Benthos is stopped or restarting.
				emptyObserved := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentReadFSMState: protocolconverter.OperationalStateIdle,
						// DataflowComponentReadObservedState left zero:
						// ObservedBenthosServiceConfig.Input is nil.
					},
				}

				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(emptyObserved)
				Expect(matched).To(BeFalse())
				Expect(renderErr).NotTo(HaveOccurred())
				Expect(localAction.LastRenderErr()).NotTo(HaveOccurred())
			})

			It("skips the render when the observed write Benthos config is absent", func() {
				// Write-side mirror of the spec above: the DFCTypeWrite branch
				// carries its own absence check on the observed Output map, so
				// a nil Output must skip the render just like a nil Input does
				// on the read side.
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(brokenWriteDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				// Observed state with an empty spec and nil observed Output,
				// normal while Benthos is stopped or restarting.
				emptyObserved := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentWriteFSMState: protocolconverter.OperationalStateIdle,
						// DataflowComponentWriteObservedState left zero:
						// ObservedBenthosServiceConfig.Output is nil.
					},
				}

				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(emptyObserved)
				Expect(matched).To(BeFalse())
				Expect(renderErr).NotTo(HaveOccurred())
				Expect(localAction.LastRenderErr()).NotTo(HaveOccurred())
			})

			It("renders with the edit's own template variables when the observed spec lacks them", func() {
				// An edit that introduces a new variable races the control loop:
				// the observed spec keeps the pre-edit variables until the FSM
				// propagates the persisted edit (seconds, under CPU pressure).
				// The verification render must use the edit's own variables, or a
				// valid edit fails its render identically every tick and the
				// fail-fast abort rolls it back.
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(newVariablePayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				// Observed spec has only the pre-edit "inject" variable, not "newvar".
				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateWithInject("benign"))

				Expect(renderErr).NotTo(HaveOccurred())
				Expect(matched).To(BeFalse())
				Expect(localAction.LastRenderErr()).NotTo(HaveOccurred())
			})

			It("fires the generic rollout_failed Sentry event on a plain timeout", func() {
				// The render succeeds every tick (benign inject) but the observed
				// config never matches, so the rollout runs into the timeout and
				// rolls back. This pre-existing path must keep its error-level
				// generic Sentry event: only the render-failure abort paths, which
				// fire their own dedicated events, suppress it.
				updateSnapshot(observedStateWithInject("benign"))
				startCollector(nil)

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				localAction.SetTickInterval(tick)
				localAction.SetAwaitTimeout(500 * time.Millisecond)
				localAction.SetFSMLogger(sentryRec)

				Expect(localAction.Parse(brokenReadDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				_, _, execErr := localAction.Execute()
				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("did not become"))

				Expect(sentryRec.eventNames()).To(ContainElement("edit_protocol_converter_rollout_failed"))

				// Drain the collector before the spec ends: without this, the
				// terminal reply may still sit in the channel buffer when the
				// next spec's BeforeEach resets the shared replies slice, and
				// the still-running collector goroutine then leaks this spec's
				// ErrRetryRollbackTimeout reply into the next spec's replies.
				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrRetryRollbackTimeout))
			})

			It("aborts on writeDFC-only render failure, rolls back, and reports ErrConfigFileInvalid", func() {
				// The five existing fail-fast specs exercise the readDFC path.
				// This spec covers the writeDFC-only tuple path in
				// compareSingleDFCConfig (DFCTypeWrite branch): the Output
				// presence check gates the render, and a broken template in
				// the output map produces the same column-0 YAML-termination
				// failure as the read-side trick.

				// observedStateWithWriteInject mirrors observedStateWithInject
				// but populates the write side: DataflowComponentWriteFSMState
				// set and ObservedBenthosServiceConfig.Output non-nil so that
				// compareSingleDFCConfig reaches the render path.
				observedStateWithWriteInject := func(inject string) *protocolconverter.ProtocolConverterObservedStateSnapshot {
					output := map[string]interface{}{"http_client": map[string]interface{}{}}
					return &protocolconverter.ProtocolConverterObservedStateSnapshot{
						ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
							Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
								ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
									NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
										Target: "localhost",
										Port:   "102",
									},
								},
							},
							Variables: variables.VariableBundle{
								User: map[string]interface{}{"inject": inject},
							},
							Location: map[string]string{"0": "test-enterprise"},
						},
						ServiceInfo: protocolconvertersvc.ServiceInfo{
							DataflowComponentWriteFSMState: protocolconverter.OperationalStateIdle,
							DataflowComponentWriteObservedState: dfcfsm.DataflowComponentObservedState{
								ServiceInfo: dfcsvc.ServiceInfo{
									BenthosObservedState: benthosfsm.BenthosObservedState{
										ObservedBenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
											Output: output,
										},
									},
								},
							},
						},
					}
				}

				updateSnapshot(observedStateWithWriteInject("x\nbroken: ["))

				// Use a private outbound channel so this spec's replies cannot
				// be contaminated by stale messages from the previous spec's
				// collector goroutine draining the shared localOutbound.
				privateOutbound := make(chan *models.UMHMessage, 100)
				var (
					privateRepliesMu sync.Mutex
					privateReplies   []capturedReply
				)
				go func() {
					for msg := range privateOutbound {
						dec, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
						if err != nil {
							continue
						}
						payload, ok := dec.Payload.(map[string]interface{})
						if !ok {
							continue
						}
						var r capturedReply
						r.state, _ = payload["actionReplyState"].(string)
						r.message, _ = payload["actionReplyPayload"].(string)
						if v2, ok := payload["actionReplyPayloadV2"].(map[string]interface{}); ok {
							r.errorCode, _ = v2["errorCode"].(string)
						}
						privateRepliesMu.Lock()
						privateReplies = append(privateReplies, r)
						privateRepliesMu.Unlock()
					}
				}()

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, privateOutbound, mockConfig, snapshotMgr)
				localAction.SetTickInterval(tick)
				localAction.SetFSMLogger(sentryRec)
				Expect(localAction.Parse(brokenWriteDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				type execResult struct{ err error }
				ch := make(chan execResult, 1)
				start := time.Now()
				go func() {
					_, _, err := localAction.Execute()
					ch <- execResult{err}
				}()

				var result execResult
				Eventually(ch, constants.DataflowComponentWaitForActiveTimeout/2).Should(Receive(&result))
				elapsed := time.Since(start)
				execErr := result.err

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("render"))
				Expect(execErr.Error()).To(ContainSubstring("was rolled back"))
				// Abort requires at least 2 full ticks after the first failure.
				Expect(elapsed).To(BeNumerically(">=", 2*tick))

				// Wait for the terminal failure reply in the private slice.
				var final capturedReply
				Eventually(func() bool {
					privateRepliesMu.Lock()
					defer privateRepliesMu.Unlock()
					for _, r := range privateReplies {
						if r.state == string(models.ActionFinishedWithFailure) {
							final = r
							return true
						}
					}
					return false
				}, "2s").Should(BeTrue(), "expected a final ActionFinishedWithFailure reply")
				Expect(final.errorCode).To(Equal(models.ErrConfigFileInvalid))
			})

			It("streak is preserved across ticks where the manager is absent from the snapshot", func() {
				// Documents the invariant: ticks that skip the comparison
				// because the protocol converter manager is missing from the
				// snapshot leave the streak untouched. A broken render error
				// that recurs across a manager-absent gap still counts as
				// consecutive and aborts after 3 identical failures.

				countManagerReplies := func() int {
					count := 0
					for _, r := range getReplies() {
						if strings.Contains(r.message, "waiting for protocol converter manager to initialise") {
							count++
						}
					}
					return count
				}

				updateSnapshot(observedStateWithInject("x\nbroken: ["))
				startCollector(nil)

				// After the first "DFC config not yet applied" reply confirms
				// tick 1 ran (streak=1), remove the manager for at least 2
				// ticks. The loop sends "waiting for protocol converter manager
				// to initialise" on those ticks; we gate on those replies as
				// proof the gap actually happened (no sleeps). Then restore the
				// broken snapshot so the streak can resume from 1 and reach 3.
				go func() {
					defer GinkgoRecover()
					Eventually(func() int {
						return countNotYetApplied()
					}, "5s", "10ms").Should(BeNumerically(">=", 1))

					snapshotMgr.UpdateSnapshot(&fsm.SystemSnapshot{
						Managers: map[string]fsm.ManagerSnapshot{},
					})

					Eventually(func() int {
						return countManagerReplies()
					}, "5s", "10ms").Should(BeNumerically(">=", 2))

					updateSnapshot(observedStateWithInject("x\nbroken: ["))
				}()

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout / 2)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("render"))
				Expect(execErr.Error()).To(ContainSubstring("was rolled back"))

				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrConfigFileInvalid))

				// Confirm the gap was observable: at least 2 manager-absent
				// ticks appeared before the streak completed.
				Expect(countManagerReplies()).To(BeNumerically(">=", 2),
					"at least 2 manager-gap ticks must be visible before abort")

				// Prove the streak SURVIVED the gap, not merely that the abort
				// eventually happened. Pre-gap the streak reached 1. If it
				// survives, the first post-gap tick raises it to 2 (one "DFC
				// config not yet applied" reply) and the next tick is the 3rd
				// identical failure, which sends "Rolling back..." instead.
				// A streak RESET by the gap would rebuild from 0 and emit 2
				// "not yet applied" replies after the gap. Count the replies
				// following the LAST manager-gap reply: exactly 1.
				allReplies := getReplies()
				lastManagerIdx := -1
				for i, r := range allReplies {
					if strings.Contains(r.message, "waiting for protocol converter manager to initialise") {
						lastManagerIdx = i
					}
				}
				Expect(lastManagerIdx).To(BeNumerically(">=", 0))
				postGapNotYet := 0
				for _, r := range allReplies[lastManagerIdx+1:] {
					if strings.Contains(r.message, "DFC config not yet applied") {
						postGapNotYet++
					}
				}
				Expect(postGapNotYet).To(Equal(1),
					"streak must survive the gap: exactly 1 not-yet-applied tick after the gap before the 3rd identical failure aborts")
			})
		})

		It("should handle config manager get config failure", func() {
			// Set up mock to fail on GetConfig
			mockConfig.WithConfigError(errors.New("mock get config failure"))

			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: http://example.com",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get current configuration: mock get config failure"))
			Expect(metadata).To(BeNil())
		})
	})

	// I1 — production switch path is safe end-to-end.
	//
	// Before the ENG-4959 wire-up fix, an EditProtocolConverter action built by
	// the production switch had a nil fsmLogger. Any code path in Execute()
	// that hits a.fsmLogger.SentryError (edit-protocolconverter.go:253, 267,
	// 291, 305, 319, 591, 860, 866) panicked at runtime. This test drives a
	// real Execute() failure through HandleActionMessage (the switch path,
	// NOT NewEditProtocolConverterAction, which already wired fsmLogger via
	// its constructor) by configuring the mock config manager to fail at
	// AtomicEditProtocolConverter. The persist failure hits a.fsmLogger.
	// SentryError at edit-protocolconverter.go:305. The pre-fix code crashed
	// the whole umh-core process here; this test asserts the action now
	// completes with ActionFinishedWithFailure instead.
	Describe("HandleActionMessage end-to-end (switch path)", func() {
		It("does not panic on AtomicEditProtocolConverter failure inside Execute", func() {
			// Force AtomicEditProtocolConverter to fail so Execute reaches
			// edit-protocolconverter.go:305, which calls a.fsmLogger.
			// SentryError. Without the wire-up fix, a.fsmLogger is nil here
			// and the SentryError call panics with a nil-pointer dereference,
			// killing the process.
			mockConfig.WithAtomicEditProtocolConverterError(errors.New("mock persist failure"))

			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"state": "active",
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: http://example.com",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			actionMsg := models.ActionMessagePayload{
				ActionType:    models.EditProtocolConverter,
				ActionUUID:    uuid.New(),
				ActionPayload: payload,
			}

			localOut := make(chan *models.UMHMessage, 16)
			done := make(chan struct{})

			go func() {
				defer GinkgoRecover()
				defer close(done)
				actions.HandleActionMessage(
					instanceUUID,
					actionMsg,
					userEmail,
					localOut,
					"",
					nil,
					uuid.Nil,
					nil,
					mockConfig,
				)
			}()

			Eventually(done, "3s").Should(BeClosed(), "HandleActionMessage should complete without panicking (ENG-4959)")

			// Drain the reply channel and assert at least one reply carries
			// the failure state. This pins the test to the failure branch:
			// if a future refactor accidentally turns Execute into a success
			// path here, the assertion fires.
			var sawFailure bool
			for len(localOut) > 0 {
				msg := <-localOut
				dec, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
				if err != nil {
					continue
				}
				reply, ok := dec.Payload.(map[string]interface{})
				if !ok {
					continue
				}
				if state, _ := reply["actionReplyState"].(string); state == string(models.ActionFinishedWithFailure) {
					sawFailure = true
				}
			}
			Expect(sawFailure).To(BeTrue(), "expected at least one ActionFinishedWithFailure reply")

			// Pin the integration test to the actual crash path. Without
			// this assertion, a future Parse-leniency change could let the
			// test pass on a happy-path or early-validation failure without
			// ever reaching the a.fsmLogger.SentryError call inside
			// Execute() — the very call site that PR #2546's missing field
			// would have crashed on. AtomicEditProtocolConverter is the
			// gate we must cross to exercise the regression.
			Expect(mockConfig.AtomicEditProtocolConverterCalled).To(BeTrue(),
				"I1 must reach AtomicEditProtocolConverter to exercise the real fsmLogger crash path")
		})
	})
})
