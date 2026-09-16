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
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUStat, Outcome: cpuhealth.ReadOK},
			cpuhealth.ReadResult{Op: cpuhealth.OpProcStat, Outcome: cpuhealth.ReadOK},
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUMax, Outcome: cpuhealth.ReadOK},
		))

		Expect(got).To(BeEmpty(), "a healthy container is why Poll can call this every tick")
	})

	It("finds nothing in a read that never ran", func() {
		got := failedReads(withReads(
			cpuhealth.ReadResult{Op: cpuhealth.OpProcStat, Outcome: cpuhealth.ReadNotAttempted},
		))

		Expect(got).To(BeEmpty(), "one failure stops later reads; reporting those splits one cause into several issues")
	})

	It("ignores the reads that ride on another read's event", func() {
		got := failedReads(withReads(
			cpuhealth.ReadResult{Op: cpuhealth.OpCgroupControllers, Outcome: cpuhealth.ReadMissing},
			cpuhealth.ReadResult{Op: cpuhealth.OpProcSelfCgroup, Outcome: cpuhealth.ReadMissing},
			cpuhealth.ReadResult{Op: cpuhealth.OpBaseDir, Outcome: cpuhealth.ReadMissing},
		))

		Expect(got).To(BeEmpty(), "these three are fields on somebody else's event and mint none of their own")
	})

	It("excuses an absent cpu.pressure but not one that will not open", func() {
		Expect(failedReads(withReads(
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUPressure, Outcome: cpuhealth.ReadMissing},
		))).To(BeEmpty(), "a kernel without PSI serves no cpu.pressure at all")

		Expect(failedReads(withReads(
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUPressure, Outcome: cpuhealth.ReadPermissionDenied},
		))).To(HaveLen(1), "a cpu.pressure that exists and will not open is a real failure")
	})

	It("calls the tick voided only for a cpu.stat that could not be read", func() {
		unreadable := failedReads(withReads(
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUStat, Outcome: cpuhealth.ReadMissing},
		))
		Expect(unreadable).To(HaveLen(1))
		Expect(unreadable[0].Message).To(Equal(sampleFailedTag))

		// Read fine, no usage figure: the sample survives without a usage rate.
		valueless := failedReads(withReads(
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUStat, Outcome: cpuhealth.ReadEmpty},
		))
		Expect(valueless).To(HaveLen(1))
		Expect(valueless[0].Message).To(Equal(readFailedTag))
	})

	It("leaves a sibling failure under its own message when cpu.stat voided the tick", func() {
		got := failedReads(withReads(
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUPressure, Outcome: cpuhealth.ReadPermissionDenied},
			cpuhealth.ReadResult{Op: cpuhealth.OpCPUStat, Outcome: cpuhealth.ReadMissing},
		))

		Expect(got).To(HaveLen(2))
		Expect(got[0].Message).To(Equal(readFailedTag), "cpu.pressure cost one signal, not the sample")
		Expect(got[1].Message).To(Equal(sampleFailedTag))
	})
})

// The reported path must be the path the sampler opened, which is not the same
// as the package's default tree: NewLinuxSampler takes the base, so an instance
// built on another one reports that one.
var _ = Describe("a failure report names the tree the sample was read from", func() {
	fieldsOf := func(sample cpuhealth.Sample, op cpuhealth.ReadOp) map[string]any {
		kv := map[string]any{}
		for _, f := range readFailureFields(sample, readFailure{Op: op}, 0, 0) {
			kv[f.Key] = f.Value
		}

		return kv
	}

	It("builds the path from the sample's base, not from the package constant", func() {
		kv := fieldsOf(cpuhealth.Sample{Troubleshooting: cpuhealth.ReadTroubleshooting{Base: "/custom/tree"}}, cpuhealth.OpCPUStat)

		Expect(kv).To(HaveKeyWithValue("path", "/custom/tree/cpu.stat"))
		Expect(kv).To(HaveKeyWithValue("cgroup_base", "/custom/tree"))
	})

	It("leaves a machine-wide file absolute whatever the base is", func() {
		kv := fieldsOf(cpuhealth.Sample{Troubleshooting: cpuhealth.ReadTroubleshooting{Base: "/custom/tree"}}, cpuhealth.OpProcStat)

		Expect(kv).To(HaveKeyWithValue("path", "/proc/stat"))
	})

	It("still names the default tree for a sampler built on it", func() {
		kv := fieldsOf(cpuhealth.Sample{Troubleshooting: cpuhealth.ReadTroubleshooting{Base: cgroupBase}}, cpuhealth.OpCPUMax)

		Expect(kv).To(HaveKeyWithValue("path", cgroupBase+"/cpu.max"))
	})
})
