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
					"destination": map[string]interface{}{
						"protocol": "kafka",
						"code":     "addresses:\n- localhost:9092\n",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*",
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetProtocolConverterUUID()).To(Equal(pcUUID))
			Expect(action.GetDFCType()).To(Equal("write"))

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.Destination.Protocol).To(Equal("kafka"))
			Expect(writeCfg.Source.Topics).To(Equal("umh.v1.factory.*"))
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*",
					},
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*\numh.v1.plant.*",
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetDFCType()).To(Equal("write"))

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.Source.Topics).To(Equal("umh.v1.factory.*\numh.v1.plant.*"))
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.{{ .location_path }}.{{ .IP }}.*",
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.Source.Topics).To(Equal("umh.v1.{{ .location_path }}.{{ .IP }}.*"))
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*",
					},
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					// no source/topics
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("input topic"))
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

		It("should store Source.Topics in typed write config when adding write DFC", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.line-1.*\numh.v1.factory.line-2.*",
					},
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
			Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Source.Topics).To(Equal("umh.v1.factory.line-1.*\numh.v1.factory.line-2.*"))
		})

		Context("when protocol converter already has both DFCs configured", func() {
			// overwrite the BeforeEach config with one that has both DFCs populated
			BeforeEach(func() {
				existingReadInput := map[string]interface{}{
					"http_client": map[string]interface{}{
						"url": "http://original.example.com",
					},
				}

				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: existingReadInput,
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
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
						"destination": map[string]interface{}{
							"protocol": "kafka",
							"code":     "addresses:\n- localhost:9092\n",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.plant.*",
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

				// Write DFC output must have changed — new format stores directly without wrapper
				writeDestType := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Destination.Protocol
				Expect(writeDestType).To(Equal("kafka"))

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
				writeDestType := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Destination.Protocol
				Expect(writeDestType).To(Equal("stdout"))

				// Source.Topics must be untouched
				Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Source.Topics).To(Equal("umh.v1.factory.*"))
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
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
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
			Expect(err.Error()).To(ContainSubstring("bridge with UUID"))
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
						"destination": map[string]interface{}{
							"protocol": "stdout",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.factory.*",
						},
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
						"destination": map[string]interface{}{
							"protocol": "stdout",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.plant.*", // different topic
						},
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
						"destination": map[string]interface{}{"protocol": "stdout"},
						"source":      map[string]interface{}{"topics": "umh.v1.factory.*"},
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
						"destination": map[string]interface{}{"protocol": "stdout"},
						"source":      map[string]interface{}{"topics": "umh.v1.factory.*"},
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
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
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
						"destination": map[string]interface{}{
							"protocol": "kafka",
							"code":     "addresses:\n- localhost:9092\n",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.plant.*",
						},
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
				writeDestType := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Destination.Protocol
				Expect(writeDestType).To(Equal("kafka"))

				// Source.Topics updated
				Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Source.Topics).To(Equal("umh.v1.plant.*"))
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
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
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
				// Dedicated outbound so the no-fail-fast Execute goroutine
				// cannot bleed replies into the next spec's collector.
				localOutbound = make(chan *models.UMHMessage, 100)
				replyMu.Lock()
				replies = nil
				replyMu.Unlock()
			})

			// observedStateWithInject: a multi-line inject value expands to a
			// column-0 line inside a block scalar, breaking YAML unmarshal.
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
					if strings.Contains(r.message, "not yet applied") {
						count++
					}
				}

				return count
			}

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
						if onProgress != nil && strings.Contains(r.message, "not yet applied") {
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
							// column-0 expansion of {{ .inject }} terminates the block scalar
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

			brokenWriteDFCPayload := func() map[string]interface{} {
				return map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"writeDFC": map[string]interface{}{
						"destination": map[string]interface{}{
							"protocol": "http_client",
							"code":     "line1\n{{ .inject }}",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.factory.*",
						},
					},
				}
			}

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

				elapsed, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout / 2)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("render"))
				Expect(execErr.Error()).To(ContainSubstring("was restored"))
				Expect(elapsed).To(BeNumerically(">=", 2*tick))

				Expect(mockConfig.AtomicEditProtocolConverterCallCount).To(Equal(2))
				Expect(mockConfig.AtomicEditProtocolConverterLastUUID).To(Equal(pcUUID))
				Expect(mockConfig.AtomicEditProtocolConverterLastConfig.Name).To(Equal(pcName))
				Expect(mockConfig.AtomicEditProtocolConverterLastConfig.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig).
					To(Equal(dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}))

				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrConfigFileInvalid))
				Expect(final.message).To(ContainSubstring("bridge '" + pcName + "'"))
				Expect(final.message).To(ContainSubstring("was restored to its previous working configuration"))
				Expect(final.message).To(ContainSubstring("read flow config isn't valid YAML"))
				Expect(final.message).To(ContainSubstring("Fix the highlighted line and try again"))
				Expect(final.message).To(ContainSubstring("yaml:"))

				Expect(countNotYetApplied()).To(Equal(2),
					"fail-fast threshold moved: expected maxIdenticalRenderFails-1 progress ticks before rollback")

				rollingBack := 0
				for _, r := range getReplies() {
					if strings.Contains(r.message, "persistent render failure detected. Rolling back...") {
						rollingBack++
					}
				}
				Expect(rollingBack).To(Equal(1))

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

				go func() {
					defer GinkgoRecover()
					for n := 1; n <= 5; n++ {
						Eventually(func() int {
							return countNotYetApplied()
						}, "5s", "10ms").Should(BeNumerically(">=", n))
						updateSnapshot(observedStateWithInject(fmt.Sprintf("x\nbroken%d: [", n)))
					}
				}()

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout - 5*time.Second)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("was restored"))

				finalFailureReply()

				Expect(countNotYetApplied()).To(BeNumerically(">=", 7))

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

				var benignFrom, benignTo int

				go func() {
					defer GinkgoRecover()
					Eventually(func() int {
						return countNotYetApplied()
					}, "5s", "10ms").Should(BeNumerically(">=", 1))
					n := countNotYetApplied()
					replyMu.Lock()
					benignFrom = n
					replyMu.Unlock()
					updateSnapshot(observedStateWithInject("benign"))

					Eventually(func() int {
						return countNotYetApplied()
					}, "5s", "10ms").Should(BeNumerically(">=", 2))
					n = countNotYetApplied()
					replyMu.Lock()
					benignTo = n
					replyMu.Unlock()
					updateSnapshot(observedStateWithInject(brokenInject))
				}()

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout - 5*time.Second)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("was restored"))

				finalFailureReply()

				Expect(countNotYetApplied()).To(BeNumerically(">=", 4))

				rollingBack := 0
				for _, r := range getReplies() {
					if strings.Contains(r.message, "persistent render failure detected. Rolling back...") {
						rollingBack++
					}
				}
				Expect(rollingBack).To(Equal(1))

				replyMu.Lock()
				from, to := benignFrom, benignTo
				replyMu.Unlock()
				Expect(to).To(BeNumerically(">", from),
					"the benign phase must contribute at least one not-yet-applied reply")
				var notYet []capturedReply
				for _, r := range getReplies() {
					if strings.Contains(r.message, "not yet applied") {
						notYet = append(notYet, r)
					}
				}
				for _, r := range notYet[from:to] {
					Expect(r.message).NotTo(ContainSubstring("Render failed"))
				}
			})

			It("returns the retryable rollback-failed code when the rollback itself fails", func() {
				updateSnapshot(observedStateWithInject("x\nbroken: ["))
				startCollector(nil)

				mockConfig.WithAtomicEditProtocolConverterError(errors.New("simulated config write failure")).
					WithAtomicEditProtocolConverterFailOnCall(2)

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout / 2)

				Expect(execErr).To(HaveOccurred())
				Expect(execErr.Error()).To(ContainSubstring("automatic rollback also failed"))
				Expect(mockConfig.AtomicEditProtocolConverterCallCount).To(Equal(2))

				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrRetryRollbackTimeout))
				Expect(final.message).To(ContainSubstring("bridge '" + pcName + "'"))
				Expect(final.message).To(ContainSubstring("couldn't be updated"))
				Expect(final.message).To(ContainSubstring("automatic rollback also failed"))
				Expect(final.message).To(ContainSubstring("may need manual recovery"))
				Expect(final.message).To(ContainSubstring("read flow config isn't valid YAML"))
				Expect(final.message).To(ContainSubstring("Fix the highlighted line and try again"))
				Expect(final.message).To(ContainSubstring("yaml:"))

				events := sentryRec.eventNames()
				Expect(events).To(ContainElement("edit_protocol_converter_render_failure_rollback_failed"))
				Expect(events).NotTo(ContainElement("edit_protocol_converter_rollout_failed"))
				failed := sentryRec.eventsNamed("edit_protocol_converter_render_failure_rollback_failed")
				Expect(failed).To(HaveLen(1))
				Expect(failed[0].fieldKeys()).To(ConsistOf(
					"protocolConverter", "protocolConverterUUID", "renderErr"))
			})

			It("keeps the render error sticky for the timeout message and clears it on the next successful render", func() {
				updateSnapshot(observedStateWithInject("benign"))

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(brokenReadDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateWithInject("x\nbroken: ["))
				Expect(matched).To(BeFalse())
				Expect(renderErr).To(HaveOccurred())
				Expect(localAction.LastRenderErr()).To(Equal(renderErr))

				matched, renderErr = localAction.CompareProtocolConverterDFCConfig(nil)
				Expect(matched).To(BeFalse())
				Expect(renderErr).NotTo(HaveOccurred())
				Expect(localAction.LastRenderErr()).To(HaveOccurred())

				matched, renderErr = localAction.CompareProtocolConverterDFCConfig(observedStateWithInject("benign"))
				Expect(matched).To(BeFalse())
				Expect(renderErr).NotTo(HaveOccurred())
				Expect(localAction.LastRenderErr()).NotTo(HaveOccurred())
			})

			It("skips the render when the observed read Benthos config is absent", func() {
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(brokenReadDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

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
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(brokenWriteDFCPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

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
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)
				Expect(localAction.Parse(newVariablePayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateWithInject("benign"))

				Expect(renderErr).NotTo(HaveOccurred())
				Expect(matched).To(BeFalse())
				Expect(localAction.LastRenderErr()).NotTo(HaveOccurred())
			})

			It("fires the generic rollout_failed Sentry event on a plain timeout", func() {
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

				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrRetryRollbackTimeout))
			})

			It("aborts on writeDFC-only render failure, rolls back, and reports ErrConfigFileInvalid", func() {
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
				Expect(execErr.Error()).To(ContainSubstring("was restored"))
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
				Expect(final.message).To(ContainSubstring("write flow config isn't valid YAML"))
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
						if strings.Contains(r.message, "waiting for bridge manager to initialise") {
							count++
						}
					}

					return count
				}

				updateSnapshot(observedStateWithInject("x\nbroken: ["))
				startCollector(nil)

				// After the first "not yet applied" reply confirms tick 1 ran
				// (streak=1), remove the manager for at least 2 ticks. The
				// loop sends "waiting for bridge manager to
				// initialise" on those ticks; we gate on those replies as
				// proof the gap actually happened (no sleeps). Then restore
				// the broken snapshot so the streak can resume from 1 and
				// reach 3.
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
				Expect(execErr.Error()).To(ContainSubstring("was restored"))

				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrConfigFileInvalid))

				// Confirm the gap was observable: at least 2 manager-absent
				// ticks appeared before the streak completed.
				Expect(countManagerReplies()).To(BeNumerically(">=", 2),
					"at least 2 manager-gap ticks must be visible before abort")

				// Prove the streak SURVIVED the gap, not merely that the abort
				// eventually happened. Pre-gap the streak reached 1. If it
				// survives, the first post-gap tick raises it to 2 (one
				// "not yet applied" reply) and the next tick is the 3rd
				// identical failure, which sends "Rolling back..." instead.
				// A streak RESET by the gap would rebuild from 0 and emit 2
				// "not yet applied" replies after the gap. Count the replies
				// following the LAST manager-gap reply: exactly 1.
				allReplies := getReplies()
				lastManagerIdx := -1
				for i, r := range allReplies {
					if strings.Contains(r.message, "waiting for bridge manager to initialise") {
						lastManagerIdx = i
					}
				}
				Expect(lastManagerIdx).To(BeNumerically(">=", 0))
				postGapNotYet := 0
				for _, r := range allReplies[lastManagerIdx+1:] {
					if strings.Contains(r.message, "not yet applied") {
						postGapNotYet++
					}
				}
				Expect(postGapNotYet).To(Equal(1),
					"streak must survive the gap: exactly 1 not-yet-applied tick after the gap before the 3rd identical failure aborts")
			})

			It("names the render failure in the per-tick progress replies, without the snippet", func() {
				// The original incident showed an empty status reason for 30s; the
				// user must see the render failure from the first failing tick.
				// The full rendered-output snippet stays out of the per-tick
				// replies and arrives once, in the terminal message.
				updateSnapshot(observedStateWithInject("x\nbroken: ["))
				startCollector(nil)

				_, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout / 2)
				Expect(execErr).To(HaveOccurred())

				var progressWithRender []capturedReply
				for _, r := range getReplies() {
					if strings.Contains(r.message, "not yet applied") {
						Expect(r.message).To(ContainSubstring("Render failed: "))
						Expect(r.message).To(ContainSubstring("yaml:"))
						Expect(r.message).NotTo(ContainSubstring("rendered output"))
						progressWithRender = append(progressWithRender, r)
					}
				}
				Expect(progressWithRender).NotTo(BeEmpty())

				// The terminal message keeps the full error including the snippet.
				Expect(execErr.Error()).To(ContainSubstring("rendered output"))
			})

			It("render-failure fail-fast: all four observables hold in a single run", func() {
				updateSnapshot(observedStateWithInject("x\nbroken: ["))
				startCollector(nil)

				elapsed, execErr := runEdit(constants.DataflowComponentWaitForActiveTimeout / 2)

				Expect(execErr).To(HaveOccurred())
				Expect(elapsed).To(BeNumerically(">=", 2*tick))

				Expect(mockConfig.AtomicEditProtocolConverterCallCount).To(Equal(2))
				Expect(mockConfig.AtomicEditProtocolConverterLastUUID).To(Equal(pcUUID))
				Expect(mockConfig.AtomicEditProtocolConverterLastConfig.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig).
					To(Equal(dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}))

				Expect(execErr.Error()).To(ContainSubstring("render"))
				Expect(execErr.Error()).To(ContainSubstring("was restored"))

				final := finalFailureReply()
				Expect(final.errorCode).To(Equal(models.ErrConfigFileInvalid))
				Expect(final.message).To(ContainSubstring("bridge '" + pcName + "'"))
				Expect(final.message).To(ContainSubstring("was restored to its previous working configuration"))
				Expect(final.message).To(ContainSubstring("read flow config isn't valid YAML"))
				Expect(final.message).To(ContainSubstring("Fix the highlighted line and try again"))
				Expect(final.message).To(ContainSubstring("rendered output around the reported line"))

				var progressWithRender []capturedReply
				for _, r := range getReplies() {
					if strings.Contains(r.message, "not yet applied") {
						Expect(r.message).To(ContainSubstring("Render failed: "))
						Expect(r.message).To(ContainSubstring("yaml:"))
						Expect(r.message).NotTo(ContainSubstring("rendered output around the reported line"))
						progressWithRender = append(progressWithRender, r)
					}
				}
				Expect(progressWithRender).NotTo(BeEmpty())

				events := sentryRec.eventNames()
				Expect(events).To(ContainElement("edit_protocol_converter_render_failure_rolled_back"))
				Expect(events).NotTo(ContainElement("edit_protocol_converter_rollout_failed"))
			})

			// ── ENG-5103 templateVars/connectionIP divergence specs ──────────────

			observedStateWithIPPort := func(observedIP string) *protocolconverter.ProtocolConverterObservedStateSnapshot {
				// The outer "input" wrapper is produced by yaml.Unmarshal of the
				// YAML string "input:\n  http_client:\n    url: '...'" — the key
				// "input" is preserved as the outer map key.
				renderedInput := map[string]interface{}{
					"input": map[string]interface{}{
						"http_client": map[string]interface{}{
							"url": "http://" + observedIP,
						},
					},
				}
				renderedPipeline := map[string]interface{}{
					"processors": []interface{}{
						map[string]interface{}{
							"bloblang": "root = content()",
						},
					},
				}

				return &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "localhost",
									Port:   "80",
								},
							},
							DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
								BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
									Input: map[string]interface{}{
										"http_client": map[string]interface{}{
											"url": "http://{{ .IP }}",
										},
									},
									Pipeline: map[string]interface{}{
										"processors": []interface{}{
											map[string]interface{}{
												"bloblang": "root = content()",
											},
										},
									},
								},
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{
								"IP": observedIP,
							},
						},
						Location: map[string]string{"0": "test-enterprise"},
					},
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentReadFSMState: protocolconverter.OperationalStateIdle,
						DataflowComponentReadObservedState: dfcfsm.DataflowComponentObservedState{
							ServiceInfo: dfcsvc.ServiceInfo{
								BenthosObservedState: benthosfsm.BenthosObservedState{
									ObservedBenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
										Input:    renderedInput,
										Pipeline: renderedPipeline,
									},
								},
							},
						},
					},
				}
			}

			observedStateNoIPInVars := func() *protocolconverter.ProtocolConverterObservedStateSnapshot {
				renderedInput := map[string]interface{}{
					"http_client": map[string]interface{}{
						"url": "http://example.com",
					},
				}

				return &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "localhost",
									Port:   "80",
								},
							},
							DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
								BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
									Input: map[string]interface{}{
										"http_client": map[string]interface{}{
											"url": "http://{{ .IP }}",
										},
									},
									Pipeline: map[string]interface{}{
										"processors": []interface{}{
											map[string]interface{}{
												"bloblang": "root = content()",
											},
										},
									},
								},
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{},
						},
						Location: map[string]string{"0": "test-enterprise"},
					},
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentReadFSMState: protocolconverter.OperationalStateIdle,
						DataflowComponentReadObservedState: dfcfsm.DataflowComponentObservedState{
							ServiceInfo: dfcsvc.ServiceInfo{
								BenthosObservedState: benthosfsm.BenthosObservedState{
									ObservedBenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
										Input: renderedInput,
									},
								},
							},
						},
					},
				}
			}

			connectionIPPayload := func(newIP string, extraVars []interface{}) map[string]interface{} {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   newIP,
						"port": 80,
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://{{ .IP }}'",
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
				if len(extraVars) > 0 {
					payload["templateInfo"] = map[string]interface{}{
						"variables": extraVars,
					}
				}

				return payload
			}

			It("verification render does not fail when the edit sets a connection IP that the observed spec lacks", func() {
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)

				Expect(localAction.Parse(connectionIPPayload("10.0.0.1", nil))).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateNoIPInVars())

				Expect(renderErr).NotTo(HaveOccurred(),
					"render must succeed using the edit's connectionIP, not fail with missingkey=error on {{ .IP }}")
				_ = matched
			})

			It("connection-only edit renders the desired config with the new IP, not the stale observed IP", func() {
				oldIP := "10.0.0.1"
				newIP := "10.0.0.2"

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)

				Expect(localAction.Parse(connectionIPPayload(newIP, nil))).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				matched, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateWithIPPort(oldIP))

				Expect(renderErr).NotTo(HaveOccurred(), "render must not fail")
				Expect(matched).To(BeFalse(),
					"the desired config rendered with the new IP must not match the observed config rendered with the old IP")
			})

			historianRefPayload := func() map[string]interface{} {
				return map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "10.0.0.1",
						"port": 80,
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://{{ .historian.timescale.host }}'",
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

			It("verification render succeeds when the edit references the historian and one is configured", func() {
				historianMock := config.NewMockConfigManager().WithConfig(config.FullConfig{
					Historian: &config.HistorianConfig{
						Timescale: config.TimescaleConfig{Host: "timescale.internal", Password: "secret"},
					},
				})
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, historianMock, snapshotMgr)

				Expect(localAction.Parse(historianRefPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				_, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateWithIPPort("10.0.0.1"))

				Expect(renderErr).NotTo(HaveOccurred(),
					"render must succeed against a configured historian, not fail with missingkey on {{ .historian.timescale.host }}")
			})

			It("verification render surfaces the true cause when the historian config read fails", func() {
				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, localOutbound, mockConfig, snapshotMgr)

				Expect(localAction.Parse(historianRefPayload())).To(Succeed())
				Expect(localAction.Validate()).To(Succeed())

				// Fail the config read only for the verify render, after Parse.
				mockConfig.WithConfigError(errors.New("mock get config failure"))

				_, renderErr := localAction.CompareProtocolConverterDFCConfig(observedStateWithIPPort("10.0.0.1"))

				Expect(renderErr).To(HaveOccurred())
				// The read failure must not masquerade as a missingkey / invalid-YAML error.
				Expect(renderErr.Error()).To(ContainSubstring("failed to read current config for historian variables"))
			})

		})

		Context("persisting merged user variables", func() {
			editPayload := func(ip string, port int, vars []interface{}) map[string]interface{} {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   ip,
						"port": port,
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://{{ .IP }}'",
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
				if len(vars) > 0 {
					payload["templateInfo"] = map[string]interface{}{
						"variables": vars,
					}
				}

				return payload
			}

			persistedUserVars := func() map[string]any {
				return mockConfig.AtomicEditProtocolConverterLastConfig.ProtocolConverterServiceConfig.Variables.User
			}

			It("does not persist templateVars named location or location_path", func() {
				vars := []interface{}{
					map[string]interface{}{"label": "location", "value": "WRONG_LOCATION"},
					map[string]interface{}{"label": "location_path", "value": "wrong.path"},
					map[string]interface{}{"label": "benign", "value": "kept"},
				}
				Expect(action.Parse(editPayload("10.0.0.1", 80, vars))).To(Succeed())
				Expect(action.Validate()).To(Succeed())

				_, _, err := action.Execute()
				Expect(err).NotTo(HaveOccurred())

				user := persistedUserVars()
				Expect(user).NotTo(HaveKey("location"))
				Expect(user).NotTo(HaveKey("location_path"))
				Expect(user).To(HaveKeyWithValue("benign", "kept"))
			})

			It("persists the connection IP over a templateVar named IP", func() {
				vars := []interface{}{
					map[string]interface{}{"label": "IP", "value": "10.0.0.99"},
				}
				Expect(action.Parse(editPayload("10.0.0.1", 80, vars))).To(Succeed())
				Expect(action.Validate()).To(Succeed())

				_, _, err := action.Execute()
				Expect(err).NotTo(HaveOccurred())

				Expect(persistedUserVars()).To(HaveKeyWithValue("IP", "10.0.0.1"))
			})

			It("persists templateVars and the connection IP and PORT into the user variables", func() {
				vars := []interface{}{
					map[string]interface{}{"label": "baudRate", "value": "9600"},
				}
				Expect(action.Parse(editPayload("10.0.0.5", 8080, vars))).To(Succeed())
				Expect(action.Validate()).To(Succeed())

				_, _, err := action.Execute()
				Expect(err).NotTo(HaveOccurred())

				user := persistedUserVars()
				Expect(user).To(HaveKeyWithValue("baudRate", "9600"))
				Expect(user).To(HaveKeyWithValue("IP", "10.0.0.5"))
				Expect(user).To(HaveKeyWithValue("PORT", "8080"))
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
