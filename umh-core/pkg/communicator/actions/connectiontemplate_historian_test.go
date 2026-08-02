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

import (
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

func historianWriteDFC() dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput {
	return dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
		Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
			Protocol: "historian",
			Code:     "host: '{{ .historian.timescale.host }}'\ndata_contract_name: pump",
		},
		Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
	}
}

func standardWriteDFC() dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput {
	return dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
		Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
			Protocol: "kafka",
			Code:     "topic: t",
		},
		Source: dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.*"},
	}
}

var _ = ginkgo.Describe("writesToHistorian", func() {
	ginkgo.It("detects a write DFC whose destination protocol is the historian plugin", func() {
		gomega.Expect(writesToHistorian(historianWriteDFC())).To(gomega.BeTrue())
	})

	ginkgo.It("ignores a bridge with a different destination protocol", func() {
		gomega.Expect(writesToHistorian(standardWriteDFC())).To(gomega.BeFalse())
	})

	ginkgo.It("ignores a non-historian bridge even if its code references the historian scope", func() {
		dfc := dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
			Destination: dataflowcomponentserviceconfig.WriteConfigDestination{
				Protocol: "kafka",
				Code:     "host: '{{ .historian.timescale.host }}'",
			},
		}
		gomega.Expect(writesToHistorian(dfc)).To(gomega.BeFalse())
	})
})
