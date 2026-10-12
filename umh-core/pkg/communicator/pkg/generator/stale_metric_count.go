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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	pcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	spfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
)

// staleMetricMargin is added to the failure window before a fired verdict
// with no new failure counts as stale. The FSMv2 benthos monitor scrapes once
// per second (pollInterval in pkg/fsmv2/benthos_monitor). A fired signal
// clears on the first scrape after its window (the signal table in that
// package), so it can clear up to one scrape late. One minute covers that with
// room to spare.
const staleMetricMargin = time.Minute

// staleMetricDegradedInstanceCount counts the Benthos instances embedded in the
// flow managers and the topic browser whose failure verdict has a fired signal
// although no failure counter rose for more than the fired entry's window plus
// staleMetricMargin. A fired signal clears within that time, so the count is
// expected to stay 0; a non-zero count means the verdict keeps flows degraded
// too long. A new manager that embeds Benthos must be added here.
func staleMetricDegradedInstanceCount(snapshot fsm.SystemSnapshot) int {
	var benthos []benthosfsm.BenthosObservedState

	for _, inst := range instancesOf(snapshot, constants.DataflowcomponentManagerName) {
		if o, ok := inst.LastObservedState.(*dfcfsm.DataflowComponentObservedStateSnapshot); ok && o != nil {
			benthos = append(benthos, o.ServiceInfo.BenthosObservedState)
		}
	}

	for _, inst := range instancesOf(snapshot, constants.ProtocolConverterManagerName) {
		if o, ok := inst.LastObservedState.(*pcfsm.ProtocolConverterObservedStateSnapshot); ok && o != nil {
			// A protocol converter embeds a Benthos instance in each of its
			// dataflow components.
			benthos = append(benthos,
				o.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState,
				o.ServiceInfo.DataflowComponentWriteObservedState.ServiceInfo.BenthosObservedState)
		}
	}

	for _, inst := range instancesOf(snapshot, constants.StreamProcessorManagerName) {
		if o, ok := inst.LastObservedState.(*spfsm.ObservedStateSnapshot); ok && o != nil {
			benthos = append(benthos, o.ServiceInfo.DFCObservedState.ServiceInfo.BenthosObservedState)
		}
	}

	for _, inst := range instancesOf(snapshot, constants.TopicBrowserManagerName) {
		if o, ok := inst.LastObservedState.(*topicbrowserfsm.ObservedStateSnapshot); ok && o != nil {
			benthos = append(benthos, o.ServiceInfo.BenthosObservedState)
		}
	}

	count := 0
	for _, b := range benthos {
		v := b.ServiceInfo.BenthosStatus.BenthosMetrics.Failures
		if v != nil && len(v.Fired) > 0 && v.ObservedAt.Sub(v.LastIncreaseAt) > v.Fired[0].Window+staleMetricMargin {
			count++
		}
	}

	return count
}

func instancesOf(snapshot fsm.SystemSnapshot, managerName string) map[string]*fsm.FSMInstanceSnapshot {
	mgr, ok := fsm.FindManager(snapshot, managerName)
	if !ok {
		return nil
	}

	return mgr.GetInstances()
}
