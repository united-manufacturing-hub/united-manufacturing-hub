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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	pcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	spfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	pcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

// lastRise is the last failure-counter rise in every verdict below.
var lastRise = time.Unix(1_800_000_000, 0)

func verdictObservedAfter(sinceRise time.Duration, fired bool) *benthos_monitor.FailureVerdict {
	v := &benthos_monitor.FailureVerdict{ObservedAt: lastRise.Add(sinceRise), LastIncreaseAt: lastRise}
	if fired {
		v.Fired = []benthos_monitor.FailureEntry{{Signal: "output_error", Label: "output errors", Increase: 1, Since: lastRise, Window: constants.BenthosLogWindow}}
	}

	return v
}

func benthosWith(v *benthos_monitor.FailureVerdict) benthosfsm.BenthosObservedState {
	return benthosfsm.BenthosObservedState{ServiceInfo: benthossvc.ServiceInfo{
		BenthosStatus: benthossvc.BenthosStatus{BenthosMetrics: benthos_monitor.BenthosMetrics{Failures: v}},
	}}
}

func dfcWith(v *benthos_monitor.FailureVerdict) dfcfsm.DataflowComponentObservedState {
	return dfcfsm.DataflowComponentObservedState{ServiceInfo: dfcsvc.ServiceInfo{BenthosObservedState: benthosWith(v)}}
}

func manager(name string, observed map[string]fsm.ObservedStateSnapshot) fsm.ManagerSnapshot {
	instances := make(map[string]*fsm.FSMInstanceSnapshot, len(observed))
	for id, o := range observed {
		instances[id] = &fsm.FSMInstanceSnapshot{ID: id, LastObservedState: o}
	}

	return &fsm.BaseManagerSnapshot{Name: name, Instances: instances}
}

var _ = Describe("staleMetricDegradedInstanceCount", func() {
	It("counts each embedded Benthos whose verdict fired with no rise for more than the window plus one minute", func() {
		stale := verdictObservedAfter(12*time.Minute, true)
		snapshot := fsm.SystemSnapshot{Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: manager(constants.DataflowcomponentManagerName, map[string]fsm.ObservedStateSnapshot{
				"dfc-stale": &dfcfsm.DataflowComponentObservedStateSnapshot{ServiceInfo: dfcsvc.ServiceInfo{BenthosObservedState: benthosWith(stale)}},
				// Inside the extra minute: 10m30s is not more than 11m.
				"dfc-inside": &dfcfsm.DataflowComponentObservedStateSnapshot{ServiceInfo: dfcsvc.ServiceInfo{BenthosObservedState: benthosWith(verdictObservedAfter(10*time.Minute+30*time.Second, true))}},
				"dfc-nil":    &dfcfsm.DataflowComponentObservedStateSnapshot{ServiceInfo: dfcsvc.ServiceInfo{BenthosObservedState: benthosWith(nil)}},
				// Exactly at the window plus the margin is not more than it.
				"dfc-boundary": &dfcfsm.DataflowComponentObservedStateSnapshot{ServiceInfo: dfcsvc.ServiceInfo{BenthosObservedState: benthosWith(verdictObservedAfter(11*time.Minute, true))}},
				// A snapshot that failed to copy is a typed nil; a foreign type is skipped.
				"dfc-typed-nil": (*dfcfsm.DataflowComponentObservedStateSnapshot)(nil),
				"dfc-foreign":   &spfsm.ObservedStateSnapshot{ServiceInfo: spsvc.ServiceInfo{DFCObservedState: dfcWith(stale)}},
			}),
			constants.ProtocolConverterManagerName: manager(constants.ProtocolConverterManagerName, map[string]fsm.ObservedStateSnapshot{
				"pc-typed-nil": (*pcfsm.ProtocolConverterObservedStateSnapshot)(nil),
				// The read flow is recent, the write flow is stale.
				"pc": &pcfsm.ProtocolConverterObservedStateSnapshot{ServiceInfo: pcsvc.ServiceInfo{
					DataflowComponentReadObservedState:  dfcWith(verdictObservedAfter(2*time.Minute, true)),
					DataflowComponentWriteObservedState: dfcWith(stale),
				}},
			}),
			constants.StreamProcessorManagerName: manager(constants.StreamProcessorManagerName, map[string]fsm.ObservedStateSnapshot{
				"sp-stale":     &spfsm.ObservedStateSnapshot{ServiceInfo: spsvc.ServiceInfo{DFCObservedState: dfcWith(stale)}},
				"sp-not-fired": &spfsm.ObservedStateSnapshot{ServiceInfo: spsvc.ServiceInfo{DFCObservedState: dfcWith(verdictObservedAfter(12*time.Minute, false))}},
				"sp-typed-nil": (*spfsm.ObservedStateSnapshot)(nil),
			}),
			constants.TopicBrowserManagerName: manager(constants.TopicBrowserManagerName, map[string]fsm.ObservedStateSnapshot{
				constants.TopicBrowserInstanceName: &topicbrowserfsm.ObservedStateSnapshot{ServiceInfo: topicbrowsersvc.ServiceInfo{BenthosObservedState: benthosWith(stale)}},
			}),
		}}

		Expect(staleMetricDegradedInstanceCount(snapshot)).To(Equal(4))
	})

	It("judges a fired entry by its own window", func() {
		// 12 minutes after the last rise is stale for a 10-minute window, not
		// for a 30-minute one.
		v := verdictObservedAfter(12*time.Minute, true)
		v.Fired[0].Window = 30 * time.Minute
		snapshot := fsm.SystemSnapshot{Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: manager(constants.DataflowcomponentManagerName, map[string]fsm.ObservedStateSnapshot{
				"dfc": &dfcfsm.DataflowComponentObservedStateSnapshot{ServiceInfo: dfcsvc.ServiceInfo{BenthosObservedState: benthosWith(v)}},
			}),
		}}

		Expect(staleMetricDegradedInstanceCount(snapshot)).To(Equal(0))
	})

	It("is 0 for a snapshot with no flow managers", func() {
		Expect(staleMetricDegradedInstanceCount(fsm.SystemSnapshot{})).To(Equal(0))
	})
})
