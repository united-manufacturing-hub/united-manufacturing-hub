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

package agent_monitor

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

type AgentMonitorIdentity struct {
	ID   string
	Name string
}

type AgentMonitorDesiredState struct {
	shutdownRequested bool
}

func (d *AgentMonitorDesiredState) ShutdownRequested() bool {
	return d.shutdownRequested
}

func (d *AgentMonitorDesiredState) SetShutdownRequested(requested bool) {
	d.shutdownRequested = requested
}

type AgentMonitorObservedState struct {
	ServiceInfo *agent_monitor.ServiceInfo
	CollectedAt time.Time
}

func (o *AgentMonitorObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o *AgentMonitorObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &AgentMonitorDesiredState{
		shutdownRequested: false,
	}
}
