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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

// failedReads is the decision reportFailedReads acts on. It takes a Sample and
// nothing else.
var _ = Describe("failedReads decides which reads earn an event", func() {
	withReads := func(rs ...cpuhealth.ReadResult) cpuhealth.Sample {
		return cpuhealth.Sample{Troubleshooting: cpuhealth.ReadTroubleshooting{Reads: rs}}
	}

	It("finds nothing in a sample whose reads all succeeded", func() {
		got := failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUStat, Outcome: cpuhealth.ReadOK},
			cpuhealth.ReadResult{Operation: cpuhealth.OperationProcStat, Outcome: cpuhealth.ReadOK},
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUMax, Outcome: cpuhealth.ReadOK},
		))

		Expect(got).To(BeEmpty(), "a healthy container is why Poll can call this every tick")
	})

	It("finds nothing in a read that never ran", func() {
		got := failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationProcStat, Outcome: cpuhealth.ReadNotAttempted},
		))

		Expect(got).To(BeEmpty(), "one failure stops later reads; reporting those splits one cause into several issues")
	})

	It("ignores the reads that ride on another read's event", func() {
		got := failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCgroupControllers, Outcome: cpuhealth.ReadMissing},
			cpuhealth.ReadResult{Operation: cpuhealth.OperationProcSelfCgroup, Outcome: cpuhealth.ReadMissing},
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCgroupBaseDir, Outcome: cpuhealth.ReadMissing},
		))

		Expect(got).To(BeEmpty(), "these three are fields on somebody else's event and mint none of their own")
	})

	It("excuses an absent cpu.pressure but not one that will not open", func() {
		Expect(failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUPressure, Outcome: cpuhealth.ReadMissing},
		))).To(BeEmpty(), "a kernel without PSI serves no cpu.pressure at all")

		Expect(failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUPressure, Outcome: cpuhealth.ReadPermissionDenied},
		))).To(HaveLen(1), "a cpu.pressure that exists and will not open is a real failure")
	})

	It("calls the tick voided only for a cpu.stat that would not parse", func() {
		unparsable := failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUStat, Outcome: cpuhealth.ReadUnparsable},
		))
		Expect(unparsable).To(HaveLen(1))
		Expect(unparsable[0].Message).To(Equal(sampleFailedTag + "::unparsable"))

		// Will not open: the three readings taken from cpu.stat go absent and
		// the sample carries on, so the host is not degraded over a file it was
		// never going to have.
		unreadable := failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUStat, Outcome: cpuhealth.ReadMissing},
		))
		Expect(unreadable).To(HaveLen(1))
		Expect(unreadable[0].Message).To(Equal(readFailedTag + "::missing"))

		// Read fine, no usage figure: the sample survives without a usage rate.
		valueless := failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUStat, Outcome: cpuhealth.ReadEmpty},
		))
		Expect(valueless).To(HaveLen(1))
		Expect(valueless[0].Message).To(Equal(readFailedTag + "::empty"))
	})

	It("leaves a sibling failure under its own message when cpu.stat voided the tick", func() {
		got := failedReads(withReads(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUPressure, Outcome: cpuhealth.ReadPermissionDenied},
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUStat, Outcome: cpuhealth.ReadUnparsable},
		))

		Expect(got).To(HaveLen(2))
		Expect(got[0].Message).To(Equal(readFailedTag+"::permission_denied"), "cpu.pressure cost one signal, not the sample")
		Expect(got[1].Message).To(Equal(sampleFailedTag + "::unparsable"))
	})
})

var _ = Describe("a failure report names the file the sample read", func() {
	It("carries the path and the cgroup version the sample recorded", func() {
		sample := cpuhealth.Sample{Troubleshooting: cpuhealth.ReadTroubleshooting{
			CgroupBase:    cgroupBase,
			CgroupVersion: "v1",
			ReadPaths:     map[cpuhealth.ReadOperation]string{cpuhealth.OperationCPUMax: cgroupBase + "/cpu,cpuacct/cpu.cfs_quota_us"},
		}}

		kv := map[string]any{}
		for _, f := range readFailureFields(sample, readFailure{Operation: cpuhealth.OperationCPUMax}, 0, 0) {
			kv[f.Key] = f.Value
		}

		Expect(kv).To(HaveKeyWithValue("path", cgroupBase+"/cpu,cpuacct/cpu.cfs_quota_us"))
		Expect(kv).To(HaveKeyWithValue("cgroup_version", "v1"))
	})
})
