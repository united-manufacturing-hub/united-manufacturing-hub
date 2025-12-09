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

package protocolconverterserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

var _ = Describe("FromConnectionAndDFCServiceConfig", func() {
	Describe("DebugLevel propagation", func() {
		It("should propagate DebugLevel=true from DFC read config", func() {
			// Arrange: DFC read config has DebugLevel=true
			connection := connectionserviceconfig.ConnectionServiceConfig{}
			dfcRead := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: true,
			}
			dfcWrite := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: false,
			}

			// Act
			result := FromConnectionAndDFCServiceConfig(connection, dfcRead, dfcWrite)

			// Assert: The runtime config should have DebugLevel=true
			Expect(result.DebugLevel).To(BeTrue(), "DebugLevel should be propagated from DFC read config")
		})

		It("should propagate DebugLevel=false from DFC read config", func() {
			// Arrange: DFC read config has DebugLevel=false
			connection := connectionserviceconfig.ConnectionServiceConfig{}
			dfcRead := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: false,
			}
			dfcWrite := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: false,
			}

			// Act
			result := FromConnectionAndDFCServiceConfig(connection, dfcRead, dfcWrite)

			// Assert: The runtime config should have DebugLevel=false
			Expect(result.DebugLevel).To(BeFalse(), "DebugLevel should be propagated from DFC read config")
		})

		It("should use DFC read DebugLevel even when DFC write differs", func() {
			// Arrange: DFC read=true, DFC write=false
			connection := connectionserviceconfig.ConnectionServiceConfig{}
			dfcRead := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: true,
			}
			dfcWrite := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				DebugLevel: false,
			}

			// Act
			result := FromConnectionAndDFCServiceConfig(connection, dfcRead, dfcWrite)

			// Assert: Should use DFC read's DebugLevel
			Expect(result.DebugLevel).To(BeTrue(), "DebugLevel should come from DFC read config")
		})
	})
})
