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

package actions

// Cross-module coverage over the deploy config-builder and the runtime renderer (fast,
// in-process — not a Testcontainers integration test): a historian bridge deployed with
// NO connection must produce a connection whose Nmap target resolves
// to the configured historian (host:port from the shared historian.timescale section) —
// i.e. the exact target the connection FSM will probe. A non-historian bridge must still
// resolve to the user-supplied IP/PORT. This lives in package actions to reach the
// unexported buildProtocolConverterConfig; ginkgo/gomega are imported qualified because
// the package declares its own Label.

import (
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
)

var _ = ginkgo.Describe("historian bridge connection (deploy build + runtime render)", func() {
	ginkgo.It("resolves the connection to the configured historian when the bridge sends no connection", func() {
		payload := models.ProtocolConverter{
			Name: "historian-bridge",
			// No Connection is sent for a historian bridge.
			WriteDFCPayload: &models.WriteDFCPayload{
				DataflowComponentWriteConfigInput: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
						Protocol: "historian",
						Code:     "host: '{{ .historian.timescale.host }}'\ndata_contract_name: pump",
					},
					Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
				},
				State: "active",
			},
		}

		cfg, err := buildProtocolConverterConfig(payload)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		historian := &config.HistorianConfig{
			Timescale: config.TimescaleConfig{
				Host:     "timescale.internal",
				Password: "secret",
				Port:     6432,
			},
		}

		runtime, err := runtime_config.BuildRuntimeConfig(
			cfg.ProtocolConverterServiceConfig,
			map[string]string{}, nil, historian, "test-node", "historian-bridge",
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// The connection FSM probes this target; it must be the historian, not a user host.
		gomega.Expect(runtime.ConnectionServiceConfig.NmapServiceConfig.Target).To(gomega.Equal("timescale.internal"))
		gomega.Expect(runtime.ConnectionServiceConfig.NmapServiceConfig.Port).To(gomega.Equal(uint16(6432)))
	})

	ginkgo.It("fails with an actionable error when a historian bridge is deployed with no historian configured", func() {
		payload := models.ProtocolConverter{
			Name: "historian-bridge",
			WriteDFCPayload: &models.WriteDFCPayload{
				DataflowComponentWriteConfigInput: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
						Protocol: "historian",
						Code:     "host: '{{ .historian.timescale.host }}'\ndata_contract_name: pump",
					},
					Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
				},
				State: "active",
			},
		}

		cfg, err := buildProtocolConverterConfig(payload)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		_, err = runtime_config.BuildRuntimeConfig(
			cfg.ProtocolConverterServiceConfig,
			map[string]string{}, nil, nil, "test-node", "historian-bridge",
		)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("no valid historian: section is configured"))
	})

	ginkgo.It("resolves the connection to the user IP/PORT for a non-historian bridge", func() {
		payload := models.ProtocolConverter{
			Name:       "modbus-bridge",
			Connection: models.ProtocolConverterConnection{IP: "10.0.0.5", Port: 502},
			WriteDFCPayload: &models.WriteDFCPayload{
				DataflowComponentWriteConfigInput: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
						Protocol: "kafka",
						Code:     "topic: t",
					},
					Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
				},
				State: "active",
			},
		}

		cfg, err := buildProtocolConverterConfig(payload)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		runtime, err := runtime_config.BuildRuntimeConfig(
			cfg.ProtocolConverterServiceConfig,
			map[string]string{}, nil, nil, "test-node", "modbus-bridge",
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		gomega.Expect(runtime.ConnectionServiceConfig.NmapServiceConfig.Target).To(gomega.Equal("10.0.0.5"))
		gomega.Expect(runtime.ConnectionServiceConfig.NmapServiceConfig.Port).To(gomega.Equal(uint16(502)))
	})
})
