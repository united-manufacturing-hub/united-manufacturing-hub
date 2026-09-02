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
	"errors"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"go.uber.org/zap"
)

// ContainerFromSnapshot converts an optional FSMInstanceSnapshot into
// a models.Container, returning sensible defaults when inst == nil.
func ContainerFromSnapshot(
	inst *fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) models.Container {
	if inst == nil {
		return defaultContainer()
	}

	c, err := buildContainer(*inst, log)
	if err != nil {
		log.Error("unable to build container data", zap.Error(err))

		return defaultContainer()
	}

	return c
}

// buildContainer assumes a *valid* snapshot and fills models.Container
// with metrics and health data. It returns an error if the observed
// state is of the wrong type.
func buildContainer(
	instance fsm.FSMInstanceSnapshot,
	_ *zap.SugaredLogger,
) (models.Container, error) {
	snap, ok := instance.LastObservedState.(*container.ContainerObservedStateSnapshot)
	if !ok || snap == nil {
		return models.Container{}, errors.New("invalid observed-state")
	}

	status := snap.ServiceInfoSnapshot
	out := defaultContainer() // start with defaults, then override

	out.Health = &models.Health{
		Message:       containerHealthMessage(status),
		ObservedState: instance.CurrentState,
		DesiredState:  instance.DesiredState,
		Category:      status.OverallHealth,
	}

	// CPU / Memory / Disk (all nil-safe)
	if status.CPU != nil {
		out.CPU = status.CPU
		if out.CPU.Health == nil {
			out.CPU.Health = &models.Health{
				Message:       getContainerHealthMessage(status.CPUHealth),
				ObservedState: status.CPUHealth.String(),
				DesiredState:  models.Active.String(),
				Category:      status.CPUHealth,
			}
		}
	}

	if status.Memory != nil {
		out.Memory = status.Memory
		if out.Memory.Health == nil {
			out.Memory.Health = &models.Health{
				Message:       getContainerHealthMessage(status.MemoryHealth),
				ObservedState: status.MemoryHealth.String(),
				DesiredState:  models.Active.String(),
				Category:      status.MemoryHealth,
			}
		}
	}

	if status.Disk != nil {
		out.Disk = status.Disk
		if out.Disk.Health == nil {
			out.Disk.Health = &models.Health{
				Message:       getContainerHealthMessage(status.DiskHealth),
				ObservedState: status.DiskHealth.String(),
				DesiredState:  models.Active.String(),
				Category:      status.DiskHealth,
			}
		}
	}

	out.Hwid = status.Hwid
	out.Architecture = status.Architecture

	return out, nil
}

// defaultContainer is used whenever no snapshot data is available.
func defaultContainer() models.Container {
	return models.Container{
		Health: &models.Health{
			Message:       "Container status unknown",
			ObservedState: "unknown",
			DesiredState:  "running",
			Category:      models.Neutral,
		},
		CPU: &models.CPU{
			Health: &models.Health{
				Message:       "CPU status unknown",
				ObservedState: "unknown",
				DesiredState:  "normal",
				Category:      models.Neutral,
			},
		},
		Memory: &models.Memory{
			Health: &models.Health{
				Message:       "Memory status unknown",
				ObservedState: "unknown",
				DesiredState:  "normal",
				Category:      models.Neutral,
			},
		},
		Disk: &models.Disk{
			Health: &models.Health{
				Message:       "Disk status unknown",
				ObservedState: "unknown",
				DesiredState:  "normal",
				Category:      models.Neutral,
			},
		},
		Hwid:         "unknown",
		Architecture: models.ArchitectureAmd64,
	}
}

// containerHealthMessage says what the components themselves said, so the
// container badge repeats the specific reason instead of a category name.
//
// A degraded component names itself and carries its own message, in the words
// IsResourceLimited builds its bridge-block reason from, so a refused bridge
// and the badge describe one condition once rather than twice. Several
// degraded components stack, one per line.
//
// With nothing degraded the CPU message stands alone. Memory and disk only
// ever say "utilization normal" on a healthy tick, which repeats what the
// badge colour already shows, while the CPU message is a composed sentence
// about actual headroom.
//
// getContainerHealthMessage below is the fallback for a tick that produced no
// component message at all.
func containerHealthMessage(status container_monitor.ServiceInfo) string {
	var cpu, memory, disk *models.Health

	if status.CPU != nil {
		cpu = status.CPU.Health
	}

	if status.Memory != nil {
		memory = status.Memory.Health
	}

	if status.Disk != nil {
		disk = status.Disk.Health
	}

	components := []struct {
		health   *models.Health
		label    string
		category models.HealthCategory
	}{
		{cpu, "CPU", status.CPUHealth},
		{memory, "Memory", status.MemoryHealth},
		{disk, "Disk", status.DiskHealth},
	}

	lines := make([]string, 0, len(components))

	for _, c := range components {
		if c.category != models.Degraded || c.health == nil || c.health.Message == "" {
			continue
		}

		lines = append(lines, c.label+" degraded: "+c.health.Message)
	}

	if len(lines) > 0 {
		return strings.Join(lines, "\n")
	}

	if status.OverallHealth == models.Active && cpu != nil && cpu.Message != "" {
		return cpu.Message
	}

	return getContainerHealthMessage(status.OverallHealth)
}

// getHealthMessage is container-specific.
func getContainerHealthMessage(cat models.HealthCategory) string {
	switch cat {
	case models.Active:
		return "Container operating normally"
	case models.Degraded:
		return "Container degraded"
	default:
		return "Container status unknown"
	}
}
