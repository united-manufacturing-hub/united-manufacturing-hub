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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
)

func specWithUnrenderedPort(destinationProtocol string) protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec {
	return protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
		Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
			ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
				NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
					Target: "{{ .historian.timescale.host }}",
					Port:   "{{ .historian.timescale.port }}",
				},
			},
			DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
				Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: destinationProtocol},
			},
		},
	}
}

var _ = Describe("connectionInfoFromSpec", func() {
	var (
		logger *zap.SugaredLogger
		logs   *observer.ObservedLogs
	)

	BeforeEach(func() {
		var core zapcore.Core
		core, logs = observer.New(zapcore.WarnLevel)
		logger = zap.New(core).Sugar()
	})

	It("logs no warning for a historian bridge, whose nmap port is an unrendered template", func() {
		actions.ConnectionInfoFromSpec(specWithUnrenderedPort(dataflowcomponentserviceconfig.HistorianDestinationProtocol), logger)

		Expect(logs.All()).To(BeEmpty())
	})

	It("still warns when another bridge's nmap port does not parse", func() {
		actions.ConnectionInfoFromSpec(specWithUnrenderedPort("kafka"), logger)

		Expect(logs.FilterMessage("Failed to parse port number").Len()).To(Equal(1))
	})
})
