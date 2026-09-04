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

package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
)

func health(message string, category models.HealthCategory) *models.Health {
	return &models.Health{
		Message:       message,
		ObservedState: category.String(),
		DesiredState:  models.Active.String(),
		Category:      category,
	}
}

var _ = Describe("containerHealthMessage", func() {
	// A healthy tick on every component. The CPU message is the generated one;
	// memory and disk carry fixed literals.
	healthy := func() container_monitor.ServiceInfo {
		return container_monitor.ServiceInfo{
			CPU:           &models.CPU{Health: health("CPU healthy. This instance is using 0.2 of 2 cores (10% of its limit) and can use 1.6 more before it is marked degraded.", models.Active)},
			Memory:        &models.Memory{Health: health("Memory utilization normal", models.Active)},
			Disk:          &models.Disk{Health: health("Disk utilization normal", models.Active)},
			OverallHealth: models.Active,
			CPUHealth:     models.Active,
			MemoryHealth:  models.Active,
			DiskHealth:    models.Active,
		}
	}

	It("should carry the CPU message verbatim when everything is healthy, rather than the fixed container literal", func() {
		msg := containerHealthMessage(healthy())

		Expect(msg).To(Equal("CPU healthy. This instance is using 0.2 of 2 cores (10% of its limit) and can use 1.6 more before it is marked degraded."))
		Expect(msg).NotTo(ContainSubstring("Container operating normally"))
	})

	It("should not name memory or disk on a healthy tick, because their messages are fixed literals that say nothing the badge needs", func() {
		msg := containerHealthMessage(healthy())

		Expect(msg).NotTo(ContainSubstring("Memory utilization normal"))
		Expect(msg).NotTo(ContainSubstring("Disk utilization normal"))
	})

	It("should name the degraded component and carry its message in the words IsResourceLimited uses for its bridge-block reason", func() {
		s := healthy()
		s.OverallHealth = models.Degraded
		s.CPUHealth = models.Degraded
		s.CPU.Health = health("CPU degraded.\nTechnical Details:\nUsage 96% of capacity (degrades above 70%).", models.Degraded)

		Expect(containerHealthMessage(s)).To(Equal(
			"CPU degraded: CPU degraded.\nTechnical Details:\nUsage 96% of capacity (degrades above 70%)."))
	})

	It("should stack several degraded components one per line, in CPU, memory, disk order", func() {
		s := healthy()
		s.OverallHealth = models.Degraded
		s.CPUHealth = models.Degraded
		s.DiskHealth = models.Degraded
		s.CPU.Health = health("usage 2.1 of 2.0 cores over 60s", models.Degraded)
		s.Disk.Health = health("94% of 235 GiB used", models.Degraded)

		Expect(containerHealthMessage(s)).To(Equal(
			"CPU degraded: usage 2.1 of 2.0 cores over 60s\nDisk degraded: 94% of 235 GiB used"))
	})

	It("should fall back to the fixed literal when no component supplied a message", func() {
		Expect(containerHealthMessage(container_monitor.ServiceInfo{OverallHealth: models.Active})).
			To(Equal("Container operating normally"))
		Expect(containerHealthMessage(container_monitor.ServiceInfo{OverallHealth: models.Degraded})).
			To(Equal("Container degraded"))
		Expect(containerHealthMessage(container_monitor.ServiceInfo{OverallHealth: models.Neutral})).
			To(Equal("Container status unknown"))
	})

	It("should fall back to the fixed literal when a degraded component carries an empty message, rather than emitting a bare prefix", func() {
		s := container_monitor.ServiceInfo{
			CPU:           &models.CPU{Health: health("", models.Degraded)},
			OverallHealth: models.Degraded,
			CPUHealth:     models.Degraded,
		}

		Expect(containerHealthMessage(s)).To(Equal("Container degraded"))
	})
})

var _ = Describe("buildContainer's health message", func() {
	// The composer is unit-tested above. This spec exists so that reverting the
	// call site to getContainerHealthMessage reddens something: without it the
	// composer can be correct and unreachable.
	It("should put the composed component message on the container health, not the category literal", func() {
		out, err := buildContainer(fsm.FSMInstanceSnapshot{
			CurrentState: "active",
			DesiredState: "active",
			LastObservedState: &container.ContainerObservedStateSnapshot{
				ServiceInfoSnapshot: container_monitor.ServiceInfo{
					CPU:           &models.CPU{Health: health("usage 2.1 of 2.0 cores over 60s", models.Degraded)},
					OverallHealth: models.Degraded,
					CPUHealth:     models.Degraded,
				},
			},
		}, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(out.Health.Message).To(Equal("CPU degraded: usage 2.1 of 2.0 cores over 60s"))
		Expect(out.Health.Message).NotTo(Equal("Container degraded"))
	})
})
