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
)

var _ = Describe("the worker never fabricates", func() {
	It("reports that it could not measure, rather than a healthy zero, when the sample failed", func() {
		// "The sample failed" means Read returned a non-nil error and nothing
		// else (e.g. an unreadable cpu.stat fails the whole snapshot). On that
		// the worker stores no verdict and reports it could not measure — never
		// a healthy zero.
		d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{}, errors.New("read cpu.stat: permission denied")
		}}, 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).To(HaveOccurred(), "a whole-sample failure must surface as an error")
		Expect(status.Verdict).To(BeEmpty(),
			"no verdict is stored — a healthy zero would fabricate a measurement")
		Expect(status.Message).To(BeEmpty())
	})

	It("still reports a verdict when a signal is absent but the sample succeeded", func() {
		// A nil error with Pressure absent is an ordinary tick: the signal that
		// cannot be read is the readability path working rather than a failure,
		// so the worker must not treat it as one.
		d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
			return cpuhealth.Sample{
				Timestamp:   time.Now(),
				Quota:       diagnosis.Known(2),
				NrPeriods:   diagnosis.Known(1),
				NrThrottled: diagnosis.Known(0),
				UsageUsec:   diagnosis.Known(5000000),
				// Pressure is ABSENT (Unknown), not failed.
				Steal:       diagnosis.Known(0),
				HostBusy:    diagnosis.Known(0.5),
				Virtualized: false,
			}, nil
		}}, 4, 2)

		status, err := Poll(context.Background(), d, CPUConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Verdict).To(Equal(string(cpuhealth.StateHealthy)),
			"a nil error with one field absent judges normally, not as a failure")
	})
})
