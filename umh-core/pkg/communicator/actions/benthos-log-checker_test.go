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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

const (
	lintErrorLine        = `time="2026-10-21T12:58:08Z" level=error msg="Config lint error" @service=benthos lint="/run/service/benthos-dataflow-write-protocolconverter-historian_pump/config/benthos.yaml(14,1) compress_after must be a duration with a unit, like \"168h\", \"30m\" or \"45s\""`
	lintErrorText        = `compress_after must be a duration with a unit, like "168h", "30m" or "45s"`
	retentionLintLine    = `time="2026-10-21T12:58:08Z" level=error msg="Config lint error" @service=benthos lint="/run/service/benthos-dataflow-write-protocolconverter-historian_pump/config/benthos.yaml(15,1) retention must be a duration with a unit, like \"168h\", \"30m\" or \"45s\""`
	retentionLintText    = `retention must be a duration with a unit, like "168h", "30m" or "45s"`
	inferErrorLine       = `level=error msg="unable to infer input type from candidates: [foo]"`
	inferErrorText       = `unable to infer input type from candidates: [foo]`
	startupLine          = `level=info msg="Running main config from specified file" @service=benthos`
	constructorErrorLine = `service closing due to: failed to init output: retention: time: missing unit in duration "168"`
)

var _ = Describe("FindBenthosConfigError", func() {
	waitStart := time.Date(2026, 10, 21, 12, 58, 0, 0, time.UTC)
	loggedAfterWaitStart := waitStart.Add(time.Second)
	loggedBeforeWaitStart := waitStart.Add(-time.Second)

	logsAt := func(timestamp time.Time, contents ...string) []s6.LogEntry {
		logs := make([]s6.LogEntry, 0, len(contents))
		for _, content := range contents {
			logs = append(logs, s6.LogEntry{Timestamp: timestamp, Content: content})
		}

		return logs
	}

	It("returns the lint message without the config file location", func() {
		text, found := actions.FindBenthosConfigError(logsAt(loggedAfterWaitStart, startupLine, lintErrorLine), waitStart)

		Expect(found).To(BeTrue())
		Expect(text).To(Equal(lintErrorText))
	})

	It("returns every lint message benthos logged", func() {
		text, found := actions.FindBenthosConfigError(logsAt(loggedAfterWaitStart, lintErrorLine, retentionLintLine), waitStart)

		Expect(found).To(BeTrue())
		Expect(text).To(Equal(lintErrorText + "; " + retentionLintText))
	})

	It("returns a repeated lint message once", func() {
		text, found := actions.FindBenthosConfigError(logsAt(loggedAfterWaitStart, lintErrorLine, lintErrorLine), waitStart)

		Expect(found).To(BeTrue())
		Expect(text).To(Equal(lintErrorText))
	})

	It("returns the msg field for a config error without a lint field", func() {
		text, found := actions.FindBenthosConfigError(logsAt(loggedAfterWaitStart, inferErrorLine), waitStart)

		Expect(found).To(BeTrue())
		Expect(text).To(Equal(inferErrorText))
	})

	It("ignores errors that are not config errors", func() {
		_, found := actions.FindBenthosConfigError(logsAt(loggedAfterWaitStart, startupLine, constructorErrorLine), waitStart)

		Expect(found).To(BeFalse())
	})

	It("ignores config errors logged before the wait started", func() {
		_, found := actions.FindBenthosConfigError(logsAt(loggedBeforeWaitStart, lintErrorLine), waitStart)

		Expect(found).To(BeFalse())
	})
})
