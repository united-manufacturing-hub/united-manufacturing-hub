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

package bridge

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
)

var _ = Describe("Delegation Approach", func() {
	Describe("SetState", func() {
		It("should delegate to sub-mocks correctly", func() {
			// Create mock service
			mockService := NewMockService()

			// Set up test flags
			flags := StateFlags{
				IsDFCRunning:       true,
				IsConnectionUp:     true,
				IsRedpandaRunning:  true,
				DfcFSMReadState:    dataflowcomponent.OperationalStateActive,
				DfcFSMWriteState:   dataflowcomponent.OperationalStateActive,
				ConnectionFSMState: connection.OperationalStateUp,
				PortState:          nmap.PortStateOpen,
			}

			// Call SetConverterState
			mockService.SetState("test-bridge", flags)

			// Verify that the service exists
			Expect(mockService.ExistingComponents["test-bridge"]).To(BeTrue())

			// Verify that ConverterStates was populated
			Expect(mockService.States["test-bridge"]).ToNot(BeNil())

			// Verify that the flags were stored
			storedFlags := mockService.GetState("test-bridge")
			Expect(storedFlags).ToNot(BeNil())
			Expect(storedFlags.IsDFCRunning).To(Equal(flags.IsDFCRunning))
			Expect(storedFlags.IsConnectionUp).To(Equal(flags.IsConnectionUp))
			Expect(storedFlags.DfcFSMReadState).To(Equal(flags.DfcFSMReadState))
			Expect(storedFlags.ConnectionFSMState).To(Equal(flags.ConnectionFSMState))
		})
	})

	Describe("BridgeToDFCFlags", func() {
		It("should convert flags correctly", func() {
			flags := StateFlags{
				IsDFCRunning:    true,
				DfcFSMReadState: dataflowcomponent.OperationalStateActive,
			}

			dfcFlags := BridgeToDFCFlags(flags)

			Expect(dfcFlags.IsBenthosRunning).To(BeTrue())
			Expect(dfcFlags.BenthosFSMState).To(Equal(dataflowcomponent.OperationalStateActive))
			Expect(dfcFlags.IsBenthosProcessingMetricsActive).To(BeTrue()) // Should be true when running and active
		})
	})

	Describe("BridgeToConnFlags", func() {
		It("should convert flags correctly", func() {
			flags := StateFlags{
				IsConnectionUp:     true,
				ConnectionFSMState: connection.OperationalStateUp,
			}

			connFlags := BridgeToConnFlags(flags)

			Expect(connFlags.IsNmapRunning).To(BeTrue())
			Expect(connFlags.NmapFSMState).To(Equal(connection.OperationalStateUp))
			Expect(connFlags.IsFlaky).To(BeFalse()) // Should default to false
		})
	})

	Describe("SetComponentState backward compatibility", func() {
		It("should work the same as SetState", func() {
			// Create mock service
			mockService := NewMockService()

			// Set up test flags
			flags := StateFlags{
				IsDFCRunning:       true,
				IsConnectionUp:     true,
				DfcFSMReadState:    dataflowcomponent.OperationalStateActive,
				ConnectionFSMState: connection.OperationalStateUp,
			}

			// Call the old method name (should delegate to SetConverterState)
			mockService.SetComponentState("test-bridge", flags)

			// Verify that it works the same as SetConverterState
			Expect(mockService.ExistingComponents["test-bridge"]).To(BeTrue())
			Expect(mockService.States["test-bridge"]).ToNot(BeNil())

			storedFlags := mockService.GetState("test-bridge")
			Expect(storedFlags).ToNot(BeNil())
			Expect(storedFlags.IsDFCRunning).To(Equal(flags.IsDFCRunning))
			Expect(storedFlags.IsConnectionUp).To(Equal(flags.IsConnectionUp))
		})
	})
})
