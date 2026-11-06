// Copyright 2026 UMH Systems GmbH
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

package actions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("EditProtocolConverter edited flow logs", func() {
	readFlowLogs := []s6.LogEntry{{Content: lintErrorLine}}
	writeFlowLogs := []s6.LogEntry{{Content: startupLine}}

	flowWithLogs := func(logs []s6.LogEntry) dfcfsm.DataflowComponentObservedState {
		return dfcfsm.DataflowComponentObservedState{
			ServiceInfo: dfcsvc.ServiceInfo{
				BenthosObservedState: benthosfsm.BenthosObservedState{
					ServiceInfo: benthossvc.ServiceInfo{
						BenthosStatus: benthossvc.BenthosStatus{BenthosLogs: logs},
					},
				},
			},
		}
	}

	snapshot := &protocolconverter.ProtocolConverterObservedStateSnapshot{
		ServiceInfo: protocolconvertersvc.ServiceInfo{
			DataflowComponentReadObservedState:  flowWithLogs(readFlowLogs),
			DataflowComponentWriteObservedState: flowWithLogs(writeFlowLogs),
		},
	}

	DescribeTable("returns the logs of the flows the edit changes",
		func(dfcType actions.DFCType, expectedReadLogs []s6.LogEntry, expectedWriteLogs []s6.LogEntry) {
			readLogs, writeLogs := actions.EditedFlowLogs(dfcType, snapshot)

			Expect(readLogs).To(Equal(expectedReadLogs))
			Expect(writeLogs).To(Equal(expectedWriteLogs))
		},
		Entry("both flows", actions.DFCTypeBoth, readFlowLogs, writeFlowLogs),
		Entry("the read flow", actions.DFCTypeRead, readFlowLogs, nil),
		Entry("the write flow", actions.DFCTypeWrite, nil, writeFlowLogs),
	)
})
