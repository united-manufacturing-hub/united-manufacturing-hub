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

// containerHealthMessage builds the string on the container's own Health, which
// the Management Console shows as its overall status.
//
// Each degraded component contributes one line, "CPU degraded: <its message>",
// joined by newlines. With nothing degraded, an Active container reports the CPU
// message alone: memory and disk are at "utilization normal" or "warning" then,
// which tells a reader nothing the category has not already said. Any other
// state falls back to getContainerHealthMessage.
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

	lines := make([]string, 0, 3)

	for _, line := range []string{
		degradedLine("CPU", status.CPUHealth, cpu),
		degradedLine("Memory", status.MemoryHealth, memory),
		degradedLine("Disk", status.DiskHealth, disk),
	} {
		if line != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) > 0 {
		return strings.Join(lines, "\n")
	}

	if status.OverallHealth == models.Active && cpu != nil && cpu.Message != "" {
		return cpu.Message
	}

	return getContainerHealthMessage(status.OverallHealth)
}

// degradedLine renders one component's line. It returns "" when the component
// is not degraded, and also when it carries no health or an empty message,
// because there is then nothing for the badge to quote.
func degradedLine(label string, cat models.HealthCategory, h *models.Health) string {
	if cat != models.Degraded || h == nil || h.Message == "" {
		return ""
	}

	return label + " degraded: " + h.Message
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
