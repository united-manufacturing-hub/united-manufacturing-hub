// Copyright 2026 UMH Systems GmbH
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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

const (
	lintErrorLine        = `time="2026-10-21T12:58:08Z" level=error msg="Config lint error" @service=benthos lint="/run/service/benthos-dataflow-write-protocolconverter-historian_pump/config/benthos.yaml(14,1) compress_after must be a duration with a unit, like \"168h\", \"30m\" or \"45s\""`
	lintErrorText        = `compress_after must be a duration with a unit, like "168h", "30m" or "45s"`
	inferErrorLine       = `level=error msg="unable to infer input type from candidates: [foo]"`
	startupLine          = `level=info msg="Running main config from specified file" @service=benthos`
	constructorErrorLine = `service closing due to: failed to init output: retention: time: missing unit in duration "168"`
)

var _ = Describe("FindBenthosConfigError", func() {
	logsOf := func(contents ...string) []s6.LogEntry {
		logs := make([]s6.LogEntry, 0, len(contents))
		for _, content := range contents {
			logs = append(logs, s6.LogEntry{Content: content})
		}

		return logs
	}

	It("returns the lint message without the config file location", func() {
		text, found := actions.FindBenthosConfigError(logsOf(startupLine, lintErrorLine))

		Expect(found).To(BeTrue())
		Expect(text).To(Equal(lintErrorText))
	})

	It("returns the whole line for a config error without a lint field", func() {
		text, found := actions.FindBenthosConfigError(logsOf(inferErrorLine))

		Expect(found).To(BeTrue())
		Expect(text).To(Equal(inferErrorLine))
	})

	It("ignores errors that are not config errors", func() {
		_, found := actions.FindBenthosConfigError(logsOf(startupLine, constructorErrorLine))

		Expect(found).To(BeFalse())
	})
})
