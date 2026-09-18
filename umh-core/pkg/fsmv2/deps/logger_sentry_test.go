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

package deps_test

import (
	"bytes"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/telemetry"
)

// The level and the message are what the Sentry hook reads: it forwards warn
// and above, takes the message as the event_name tag and fingerprints on it,
// and renders the error as the issue's subtitle.
var _ = Describe("Sentry reports a declared identifier", func() {
	var buf *bytes.Buffer

	warning := telemetry.Identifier{
		Tag:      "cpu::read_failed",
		Brief:    "A cgroup CPU file could not be read.",
		Severity: telemetry.SeverityWarning,
	}
	failure := telemetry.Identifier{
		Tag:      "cpu::sample_failed",
		Brief:    "No CPU sample could be taken at all.",
		Severity: telemetry.SeverityError,
	}

	BeforeEach(func() {
		buf = new(bytes.Buffer)
	})

	It("logs a warning-severity identifier at warn, with the tag as the message", func() {
		deps.NewJSONFSMLogger(buf, deps.LevelDebug).
			Sentry(warning, deps.FeatureSupportCPU, "root/cpu-1(cpu)", nil)

		line := parseLine(buf)
		Expect(line).To(HaveKeyWithValue("level", "warn"))
		Expect(line).To(HaveKeyWithValue("msg", "cpu::read_failed"))
		Expect(line).To(HaveKeyWithValue("feature", "support_cpu"))
		Expect(line).To(HaveKeyWithValue("hierarchy_path", "root/cpu-1(cpu)"))
	})

	It("logs an error-severity identifier at error", func() {
		deps.NewJSONFSMLogger(buf, deps.LevelDebug).
			Sentry(failure, deps.FeatureSupportCPU, "", errors.New("boom"))

		Expect(parseLine(buf)).To(HaveKeyWithValue("level", "error"))
	})

	It("synthesizes the brief when the caller has no error", func() {
		// Without this the event reaches Sentry with no exception and no readable
		// sentence, which is the hole the registry exists to close.
		deps.NewJSONFSMLogger(buf, deps.LevelDebug).
			Sentry(warning, deps.FeatureSupportCPU, "", nil)

		Expect(parseLine(buf)).To(HaveKeyWithValue("error", "A cgroup CPU file could not be read."))
	})

	It("keeps the caller's error when there is one", func() {
		deps.NewJSONFSMLogger(buf, deps.LevelDebug).
			Sentry(failure, deps.FeatureSupportCPU, "", errors.New("boom"))

		Expect(parseLine(buf)).To(HaveKeyWithValue("error", "boom"))
	})

	It("carries the caller's own fields", func() {
		deps.NewJSONFSMLogger(buf, deps.LevelDebug).
			Sentry(warning, deps.FeatureSupportCPU, "", nil, deps.String("path", "/sys/fs/cgroup/cpu.stat"))

		Expect(parseLine(buf)).To(HaveKeyWithValue("path", "/sys/fs/cgroup/cpu.stat"))
	})

	It("reports a zero identifier as a defect rather than a blank event", func() {
		// A zero Identifier is constructible even though the generated tree never
		// yields one. Emitting it would put an empty event_name into Sentry, which
		// collects every such bug into one unreadable issue.
		deps.NewJSONFSMLogger(buf, deps.LevelDebug).
			Sentry(telemetry.Identifier{}, deps.FeatureFSMv2, "root", nil, deps.String("caller", "x"))

		line := parseLine(buf)
		Expect(line).To(HaveKeyWithValue("msg", "telemetry::unregistered_identifier"))
		Expect(line).To(HaveKeyWithValue("level", "error"))
		Expect(line).To(HaveKeyWithValue("caller", "x"))
	})

	It("omits hierarchy_path when empty", func() {
		deps.NewJSONFSMLogger(buf, deps.LevelDebug).
			Sentry(warning, deps.FeatureSupportCPU, "", nil)

		Expect(parseLine(buf)).NotTo(HaveKey("hierarchy_path"))
	})
})
