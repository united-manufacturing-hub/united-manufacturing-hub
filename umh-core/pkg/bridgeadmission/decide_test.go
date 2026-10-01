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

package bridgeadmission_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ba "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/bridgeadmission"
)

const hint = "agent.enableResourceLimitBlocking: false"

func cores(c float64) *float64 { return &c }

func healthy() ba.Resource { return ba.Resource{Health: ba.Healthy} }

// allHealthy is an instance with proven health and a 4-core CPU limit, so the
// bridge limit is (4 - 1) * 5 = 15.
func allHealthy() ba.Input {
	return ba.Input{
		EnableResourceLimitBlocking: true,
		CPU:                         healthy(),
		Memory:                      healthy(),
		Disk:                        healthy(),
		CapacityCores:               cores(4),
		HostCores:                   32,
	}
}

var _ = Describe("Decide", func() {
	Describe("with enableResourceLimitBlocking false", func() {
		It("admits without checking anything", func() {
			in := ba.Input{
				EnableResourceLimitBlocking: false,
				CPU:                         ba.Resource{Health: ba.Degraded, Message: "cpu is full"},
				Created:                     1000,
			}

			d := ba.Decide(in)

			Expect(d.Admit).To(BeTrue())
			Expect(d.Cause).To(Equal(ba.None))
			Expect(d.MaxBridges).To(BeNil())
			Expect(d.Message()).To(BeEmpty())
		})
	})

	Describe("health", func() {
		It("refuses the zero Input, because nothing is proven", func() {
			d := ba.Decide(ba.Input{EnableResourceLimitBlocking: true})

			Expect(d.Admit).To(BeFalse())
			Expect(d.Cause).To(Equal(ba.NotProven))
			Expect(d.Reason).To(Equal("Resource health not proven yet"))
		})

		DescribeTable("refuses a degraded resource and names it",
			func(mutate func(*ba.Input), cause ba.Cause, reason string) {
				in := allHealthy()
				mutate(&in)

				d := ba.Decide(in)

				Expect(d.Admit).To(BeFalse())
				Expect(d.Cause).To(Equal(cause))
				Expect(d.Reason).To(Equal(reason))
				Expect(d.MaxBridges).To(BeNil())
			},
			Entry("CPU with a message",
				func(in *ba.Input) { in.CPU = ba.Resource{Health: ba.Degraded, Message: "cpu is full"} },
				ba.CPU, "CPU degraded: cpu is full"),
			Entry("CPU without a message",
				func(in *ba.Input) { in.CPU = ba.Resource{Health: ba.Degraded} },
				ba.CPU, "CPU resources degraded"),
			Entry("memory with a message",
				func(in *ba.Input) { in.Memory = ba.Resource{Health: ba.Degraded, Message: "memory is full"} },
				ba.Memory, "Memory degraded: memory is full"),
			Entry("memory without a message",
				func(in *ba.Input) { in.Memory = ba.Resource{Health: ba.Degraded} },
				ba.Memory, "Memory resources degraded"),
			Entry("disk with a message",
				func(in *ba.Input) { in.Disk = ba.Resource{Health: ba.Degraded, Message: "disk is full"} },
				ba.Disk, "Disk degraded: disk is full"),
			Entry("disk without a message",
				func(in *ba.Input) { in.Disk = ba.Resource{Health: ba.Degraded} },
				ba.Disk, "Disk resources degraded"),
			Entry("CPU before memory when both are degraded",
				func(in *ba.Input) {
					in.CPU = ba.Resource{Health: ba.Degraded, Message: "cpu is full"}
					in.Memory = ba.Resource{Health: ba.Degraded, Message: "memory is full"}
				},
				ba.CPU, "CPU degraded: cpu is full"),
			Entry("a degraded resource before an unknown one",
				func(in *ba.Input) {
					in.CPU = ba.Resource{Health: ba.Unproven, Message: "not measured yet"}
					in.Disk = ba.Resource{Health: ba.Degraded, Message: "disk is full"}
				},
				ba.Disk, "Disk degraded: disk is full"),
		)

		DescribeTable("refuses an unknown resource as not proven, carrying its message",
			func(mutate func(*ba.Input)) {
				in := allHealthy()
				mutate(&in)

				d := ba.Decide(in)

				Expect(d.Admit).To(BeFalse())
				Expect(d.Cause).To(Equal(ba.NotProven))
				Expect(d.Reason).To(Equal("Resource health not proven yet: instance not active yet"))
				Expect(d.MaxBridges).To(BeNil())
			},
			Entry("CPU", func(in *ba.Input) { in.CPU = ba.Resource{Health: ba.Unproven, Message: "instance not active yet"} }),
			Entry("memory", func(in *ba.Input) { in.Memory = ba.Resource{Health: ba.Unproven, Message: "instance not active yet"} }),
			Entry("disk", func(in *ba.Input) { in.Disk = ba.Resource{Health: ba.Unproven, Message: "instance not active yet"} }),
		)
	})

	Describe("bridge limit", func() {
		DescribeTable("admits while Created + WaitingBefore stay below MaxBridges",
			func(created, waitingBefore int, admit bool) {
				in := allHealthy()
				in.Created = created
				in.WaitingBefore = waitingBefore

				d := ba.Decide(in)

				Expect(d.Admit).To(Equal(admit))
				Expect(d.MaxBridges).NotTo(BeNil())
				Expect(*d.MaxBridges).To(Equal(15))
				if admit {
					Expect(d.Cause).To(Equal(ba.None))
				} else {
					Expect(d.Cause).To(Equal(ba.BridgeLimit))
					Expect(d.Reason).To(ContainSubstring("limit exceeded"))
					Expect(d.Reason).To(ContainSubstring("15 bridges maximum"))
				}
			},
			Entry("first bridge", 0, 0, true),
			Entry("the 15th bridge after 14 created", 14, 0, true),
			Entry("a 16th bridge after 15 created", 15, 0, false),
			Entry("after a restart: the 15th waiting bridge", 0, 14, true),
			Entry("after a restart: the 16th waiting bridge", 0, 15, false),
			Entry("created and waiting together at the limit", 10, 4, true),
			Entry("created and waiting together over the limit", 10, 5, false),
		)

		DescribeTable("computes the limit as (cores - 1) * 5 from the CPU limit, else the host cores",
			func(capacity *float64, hostCores, limit int) {
				in := allHealthy()
				in.CapacityCores = capacity
				in.HostCores = hostCores

				d := ba.Decide(in)

				Expect(d.MaxBridges).NotTo(BeNil())
				Expect(*d.MaxBridges).To(Equal(limit))
				Expect(d.Admit).To(Equal(limit > 0))
			},
			Entry("CPU limit of 4 cores", cores(4), 32, 15),
			Entry("CPU limit of 2.5 cores", cores(2.5), 32, 7),
			Entry("no CPU limit known: host cores", nil, 3, 10),
			Entry("CPU limit of 0: host cores", cores(0), 3, 10),
			Entry("one core leaves no room for bridges", cores(1), 32, 0),
			Entry("less than one core", cores(0.5), 32, 0),
		)
	})

	Describe("Message", func() {
		It("is the reason followed by how to start bridges anyway", func() {
			in := allHealthy()
			in.CPU = ba.Resource{Health: ba.Degraded, Message: "cpu is full"}

			d := ba.Decide(in)

			Expect(d.Message()).To(HavePrefix("CPU degraded: cpu is full"))
			Expect(d.Message()).To(ContainSubstring(hint))
		})

		DescribeTable("carries the hint on every kind of refusal",
			func(in ba.Input) {
				d := ba.Decide(in)

				Expect(d.Admit).To(BeFalse())
				Expect(d.Message()).To(HavePrefix(d.Reason))
				Expect(d.Message()).To(ContainSubstring(hint))
			},
			Entry("not proven", ba.Input{EnableResourceLimitBlocking: true}),
			Entry("memory", func() ba.Input { in := allHealthy(); in.Memory.Health = ba.Degraded; return in }()),
			Entry("disk", func() ba.Input { in := allHealthy(); in.Disk.Health = ba.Degraded; return in }()),
			Entry("bridge limit", func() ba.Input { in := allHealthy(); in.Created = 15; return in }()),
		)

		It("is empty for an admitted bridge", func() {
			Expect(ba.Decide(allHealthy()).Message()).To(BeEmpty())
		})
	})
})
