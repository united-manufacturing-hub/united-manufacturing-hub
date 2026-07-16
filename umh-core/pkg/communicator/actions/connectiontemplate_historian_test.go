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

// This test lives in package actions (not actions_test) to exercise the unexported
// connection-template selector directly. The actions package declares its own Label
// helper, which collides with a ginkgo dot-import, so ginkgo/gomega are imported qualified.

import (
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

var _ = ginkgo.Describe("connectionTemplateForWriteOutput", func() {
	ginkgo.It("targets the shared historian section when the output inherits the historian connection", func() {
		tmpl := connectionTemplateForWriteOutput(
			dataflowcomponentserviceconfig.WriteConfigDestination{
				Protocol: "historian",
				Code:     "host: '{{ .historian.timescale.host }}'\nport: {{ .historian.timescale.port }}",
			},
		)
		gomega.Expect(tmpl.NmapTemplate).NotTo(gomega.BeNil())
		gomega.Expect(tmpl.NmapTemplate.Target).To(gomega.Equal("{{ .historian.timescale.host }}"))
		gomega.Expect(tmpl.NmapTemplate.Port).To(gomega.Equal("{{ .historian.timescale.port }}"))
	})

	ginkgo.It("uses the standard IP/PORT template for a manual historian output that uses {{ .IP }}", func() {
		// The manual TimescaleDB template writes through the historian plugin but supplies
		// its own connection, so it must NOT be retargeted at the shared historian.
		tmpl := connectionTemplateForWriteOutput(
			dataflowcomponentserviceconfig.WriteConfigDestination{
				Protocol: "historian",
				Code:     "host: {{ .IP }}\nport: {{ .PORT }}",
			},
		)
		gomega.Expect(tmpl.NmapTemplate).NotTo(gomega.BeNil())
		gomega.Expect(tmpl.NmapTemplate.Target).To(gomega.Equal("{{ .IP }}"))
		gomega.Expect(tmpl.NmapTemplate.Port).To(gomega.Equal("{{ .PORT }}"))
	})

	ginkgo.It("uses the standard IP/PORT template for a non-historian write output", func() {
		tmpl := connectionTemplateForWriteOutput(
			dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "kafka", Code: "topic: t"},
		)
		gomega.Expect(tmpl.NmapTemplate).NotTo(gomega.BeNil())
		gomega.Expect(tmpl.NmapTemplate.Target).To(gomega.Equal("{{ .IP }}"))
		gomega.Expect(tmpl.NmapTemplate.Port).To(gomega.Equal("{{ .PORT }}"))
	})

	ginkgo.It("uses the standard IP/PORT template when no write output is set", func() {
		tmpl := connectionTemplateForWriteOutput(dataflowcomponentserviceconfig.WriteConfigDestination{})
		gomega.Expect(tmpl.NmapTemplate).NotTo(gomega.BeNil())
		gomega.Expect(tmpl.NmapTemplate.Target).To(gomega.Equal("{{ .IP }}"))
	})
})
