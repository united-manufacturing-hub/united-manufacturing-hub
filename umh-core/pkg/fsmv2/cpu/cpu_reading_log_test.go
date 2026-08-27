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

package fsmv2cpu

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// debugRecord captures one actual Debug invocation: its message name and its
// structured fields.
type debugRecord struct {
	msg    string
	fields map[string]any
}

// debugSpyLogger wraps NopFSMLogger and records every actual Debug invocation —
// its message and structured fields — so a test can assert on real call counts
// rather than a field. With returns the spy itself because
// deps.NewBaseDependencies enriches the logger it is given through With, and a
// pass-through to the wrapped Nop would hide every later Debug from the spy.
// Test-local: only this test observes the per-tick reading entry.
type debugSpyLogger struct {
	deps.FSMLogger
	debugs []debugRecord
}

func (l *debugSpyLogger) Debug(msg string, fields ...deps.Field) {
	rec := debugRecord{msg: msg, fields: make(map[string]any, len(fields))}
	for _, f := range fields {
		rec.fields[f.Key] = f.Value
	}

	l.debugs = append(l.debugs, rec)
}

func (l *debugSpyLogger) With(_ ...deps.Field) deps.FSMLogger { return l }

// cpuReadings returns the Debug entries the spy captured under the fixed name
// cpu_reading, whatever else the tick logged.
func cpuReadings(l *debugSpyLogger) []debugRecord {
	var readings []debugRecord
	for _, rec := range l.debugs {
		if rec.msg == "cpu_reading" {
			readings = append(readings, rec)
		}
	}

	return readings
}

// newDepsWithLogger is newDeps with an explicit logger, so a spec can observe
// what Poll writes. It lives here rather than beside newDeps because its
// previous home, cpu_admission_deadline_warning_test.go, was deleted with the
// admission window, and this spec is its only remaining caller.
func newDepsWithLogger(log deps.FSMLogger, s cpuhealth.Sampler, cores, quota float64) *CPUDeps {
	engine, err := diagnosis.NewEngine(cpuhealth.Table(cores, quota))
	Expect(err).NotTo(HaveOccurred(), "the test table must be buildable")

	return &CPUDeps{
		BaseDependencies: deps.NewBaseDependencies(log, nil, deps.Identity{ID: "cpu-test", WorkerType: WorkerType}),
		sampler:          s,
		engine:           engine,
	}
}

var _ = Describe("a completed CPU poll's reading log", func() {
	It("records the verdict it reached and the message it composed, once per completed poll and never for a failed read", func() {
		// Presence: a quiet, fully present sample on a 4-cores/2-quota box —
		// the same setup as "reports a verdict from Decide rather than a raw
		// measurement" — judges healthy, and that judgement must be on the log.
		spy := &debugSpyLogger{FSMLogger: deps.NewNopFSMLogger()}
		d := newDepsWithLogger(spy, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{
				Timestamp: time.Now(),
				Quota:     diagnosis.Known(2),
				// All signals present and quiet: nothing fires.
				NrPeriods:   diagnosis.Known(1),
				NrThrottled: diagnosis.Known(0),
				UsageUsec:   diagnosis.Known(5000000),
				Pressure:    diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
				HostBusy:    diagnosis.Known(0.5),
				Virtualized: false,
			}, nil
		}}, 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		// The recorded fields must carry what this tick actually produced, so
		// pin that the tick produced something before reading it back.
		Expect(status.Verdict).NotTo(BeEmpty(), "a completed tick must reach a verdict for one to be recorded")
		Expect(status.Message).NotTo(BeEmpty(), "a completed tick must compose a message for one to be recorded")

		readings := cpuReadings(spy)
		Expect(readings).To(HaveLen(1), "a completed poll emits exactly one cpu_reading Debug entry")
		// Both field VALUES are asserted — never just the entry's presence —
		// so an entry emitted with empty fields fails here.
		Expect(readings[0].fields["verdict"]).To(Equal(status.Verdict),
			"the entry's verdict field is the verdict the poll reached")
		Expect(readings[0].fields["message"]).To(Equal(status.Message),
			"the entry's message field is the message the poll composed")

		// Absence: a read that fails never completes a poll, so nothing may be
		// recorded for it.
		readErr := errors.New("cgroup read failed")
		spyFailed := &debugSpyLogger{FSMLogger: deps.NewNopFSMLogger()}
		dFailed := newDepsWithLogger(spyFailed, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{}, readErr
		}}, 4, 2)

		_, err = Poll(context.Background(), dFailed, CPUConfig{})
		Expect(err).To(MatchError(readErr), "the failure path ran: Poll surfaces the read error")
		Expect(cpuReadings(spyFailed)).To(BeEmpty(),
			"a poll whose read failed records no cpu_reading entry")
	})
})
