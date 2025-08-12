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

package topicbrowser_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
)

var _ = Describe("Simulator", func() {
	const (
		bufferLimit     = 100
		testBundleCount = 102
	)

	var simulator *topicbrowser.Simulator

	BeforeEach(func() {
		simulator = topicbrowser.NewSimulator()
		simulator.InitializeSimulator()
	})

	Describe("NewSimulator", func() {
		It("should create a new simulator with proper initialization", func() {
			Expect(simulator).NotTo(BeNil())

			observedState := simulator.GetSimObservedState()
			Expect(observedState).NotTo(BeNil())
			Expect(observedState.ServiceInfo.Status.BufferSnapshot.Items).To(BeEmpty())
		})

		It("should initialize with simulator enabled", func() {
			// Test that tick actually generates data (indicating simulator is enabled)
			initialState := simulator.GetSimObservedState()
			initialBufferLen := len(initialState.ServiceInfo.Status.BufferSnapshot.Items)

			simulator.Tick()

			newState := simulator.GetSimObservedState()
			Expect(newState.ServiceInfo.Status.BufferSnapshot.Items).To(HaveLen(initialBufferLen + 1))
		})
	})

	Describe("InitializeSimulator", func() {
		It("should set up initial topics correctly", func() {
			// Generate a bundle to verify topics are initialized
			bundle := simulator.GenerateNewUnsBundle()
			Expect(bundle).NotTo(BeEmpty())

			// Unmarshal and verify structure
			var unsBundle tbproto.UnsBundle
			err := proto.Unmarshal(bundle, &unsBundle)
			Expect(err).NotTo(HaveOccurred())

			Expect(unsBundle.GetUnsMap()).NotTo(BeNil())
			Expect(unsBundle.GetUnsMap().GetEntries()).To(HaveLen(2))

			// Verify the hardcoded topics exist
			foundTopic1 := false
			foundTopic2 := false

			for _, topic := range unsBundle.GetUnsMap().GetEntries() {
				if topic.GetName() == "uns.topic1" {
					foundTopic1 = true
					Expect(topic.GetLevel0()).To(Equal("corpA"))
					Expect(topic.GetLocationSublevels()).To(Equal([]string{"plant-1", "line-4", "pump-41"}))
					Expect(topic.GetDataContract()).To(Equal("uns.topic1"))
				}
				if topic.GetName() == "uns.topic2" {
					foundTopic2 = true
					Expect(topic.GetLevel0()).To(Equal("corpA"))
					Expect(topic.GetLocationSublevels()).To(Equal([]string{"plant-1", "line-4", "pump-42"}))
					Expect(topic.GetDataContract()).To(Equal("uns.topic2"))
				}
			}

			Expect(foundTopic1).To(BeTrue())
			Expect(foundTopic2).To(BeTrue())
		})
	})

	Describe("GenerateNewUnsBundle", func() {
		It("should generate valid protobuf bundle", func() {
			bundle := simulator.GenerateNewUnsBundle()
			Expect(bundle).NotTo(BeEmpty())

			var unsBundle tbproto.UnsBundle
			err := proto.Unmarshal(bundle, &unsBundle)
			Expect(err).NotTo(HaveOccurred())

			Expect(unsBundle.GetUnsMap()).NotTo(BeNil())
			Expect(unsBundle.GetUnsMap().GetEntries()).To(HaveLen(2))
			Expect(unsBundle.GetEvents()).NotTo(BeNil())
			Expect(unsBundle.GetEvents().GetEntries()).To(HaveLen(2))
			Expect(unsBundle.GetEvents().GetEntries()[0].GetProducedAtMs()).To(BeNumerically(">=", 0))
			Expect(unsBundle.GetEvents().GetEntries()[1].GetProducedAtMs()).To(BeNumerically(">=", unsBundle.GetEvents().GetEntries()[0].GetProducedAtMs()))
		})

		It("should generate events for all topics", func() {
			bundle := simulator.GenerateNewUnsBundle()

			var unsBundle tbproto.UnsBundle
			err := proto.Unmarshal(bundle, &unsBundle)
			Expect(err).NotTo(HaveOccurred())

			topicCount := len(unsBundle.GetUnsMap().GetEntries())
			eventCount := len(unsBundle.GetEvents().GetEntries())

			Expect(eventCount).To(Equal(topicCount))
		})

		It("should generate random numeric values", func() {
			bundle1 := simulator.GenerateNewUnsBundle()
			bundle2 := simulator.GenerateNewUnsBundle()

			var unsBundle1, unsBundle2 tbproto.UnsBundle
			Expect(proto.Unmarshal(bundle1, &unsBundle1)).To(Succeed())
			Expect(proto.Unmarshal(bundle2, &unsBundle2)).To(Succeed())

			// Get first event from each bundle
			event1 := unsBundle1.GetEvents().GetEntries()[0]
			event2 := unsBundle2.GetEvents().GetEntries()[0]

			Expect(event1.GetPayload().(*tbproto.EventTableEntry_Ts).Ts.GetScalarType()).To(Equal(tbproto.ScalarType_NUMERIC))
			Expect(event2.GetPayload().(*tbproto.EventTableEntry_Ts).Ts.GetScalarType()).To(Equal(tbproto.ScalarType_NUMERIC))

			val1 := event1.Payload.(*tbproto.EventTableEntry_Ts).Ts.GetValue().(*tbproto.TimeSeriesPayload_NumericValue).NumericValue.GetValue()
			val2 := event2.Payload.(*tbproto.EventTableEntry_Ts).Ts.GetValue().(*tbproto.TimeSeriesPayload_NumericValue).NumericValue.GetValue()

			// Values should be between 0 and 100
			Expect(val1).To(BeNumerically(">=", 0))
			Expect(val1).To(BeNumerically("<=", 100))
			Expect(val2).To(BeNumerically(">=", 0))
			Expect(val2).To(BeNumerically("<=", 100))
		})

		It("should use ticker value as ProducedAtMs", func() {
			// Advance ticker
			simulator.Tick()
			simulator.Tick()

			bundle := simulator.GenerateNewUnsBundle()

			var unsBundle tbproto.UnsBundle
			err := proto.Unmarshal(bundle, &unsBundle)
			Expect(err).NotTo(HaveOccurred())

			// All events should have ProducedAtMs equal to current ticker value
			for _, event := range unsBundle.GetEvents().GetEntries() {
				Expect(event.GetProducedAtMs()).To(BeNumerically(">", 0))
			}
		})
	})

	Describe("AddUnsBundleToSimObservedState", func() {
		It("should add bundle to observed state buffer", func() {
			initialState := simulator.GetSimObservedState()
			initialCount := len(initialState.ServiceInfo.Status.BufferSnapshot.Items)

			bundle := simulator.GenerateNewUnsBundle()
			simulator.AddUnsBundleToSimObservedState(bundle)

			newState := simulator.GetSimObservedState()
			Expect(newState.ServiceInfo.Status.BufferSnapshot.Items).To(HaveLen(initialCount + 1))

			addedBuffer := newState.ServiceInfo.Status.BufferSnapshot.Items[len(newState.ServiceInfo.Status.BufferSnapshot.Items)-1]
			Expect(addedBuffer.Payload).To(Equal(bundle))
			Expect(addedBuffer.Timestamp.Unix()).To(BeNumerically("~", time.Now().Unix(), 1))
		})

		It("should limit buffer to 100 entries", func() {
			// Add 102 bundles to test the limit
			for range testBundleCount {
				bundle := simulator.GenerateNewUnsBundle()
				simulator.AddUnsBundleToSimObservedState(bundle)
			}

			state := simulator.GetSimObservedState()
			Expect(state.ServiceInfo.Status.BufferSnapshot.Items).To(HaveLen(bufferLimit))
		})

		It("should remove oldest entries when buffer exceeds limit", func() {
			// Add a distinctive first bundle
			firstBundle := []byte("first-bundle")
			simulator.AddUnsBundleToSimObservedState(firstBundle)

			// Fill up the buffer
			for range bufferLimit {
				bundle := simulator.GenerateNewUnsBundle()
				simulator.AddUnsBundleToSimObservedState(bundle)
			}

			state := simulator.GetSimObservedState()

			// First bundle should be gone
			for _, buffer := range state.ServiceInfo.Status.BufferSnapshot.Items {
				Expect(buffer.Payload).NotTo(Equal(firstBundle))
			}
		})

		It("should be thread-safe", func() {
			var wg sync.WaitGroup
			numGoroutines := 10
			bundlesPerGoroutine := 5

			for range numGoroutines {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range bundlesPerGoroutine {
						bundle := simulator.GenerateNewUnsBundle()
						simulator.AddUnsBundleToSimObservedState(bundle)
					}
				}()
			}

			wg.Wait()

			state := simulator.GetSimObservedState()
			expectedCount := numGoroutines * bundlesPerGoroutine
			Expect(state.ServiceInfo.Status.BufferSnapshot.Items).To(HaveLen(expectedCount))
		})
	})

	Describe("GetSimObservedState", func() {
		It("should return current observed state", func() {
			// fill up the buffer
			for range bufferLimit {
				bundle := simulator.GenerateNewUnsBundle()
				simulator.AddUnsBundleToSimObservedState(bundle)
			}

			state := simulator.GetSimObservedState()
			Expect(state).NotTo(BeNil())
			Expect(state.ServiceInfo.Status.BufferSnapshot.Items).NotTo(BeNil())
			Expect(state.ServiceInfo.Status.BufferSnapshot.Items).To(HaveLen(bufferLimit))
		})

		It("should be thread-safe for concurrent reads", func() {
			var wg sync.WaitGroup
			numReaders := 10

			for range numReaders {
				wg.Add(1)
				go func() {
					defer wg.Done()
					state := simulator.GetSimObservedState()
					Expect(state).NotTo(BeNil())
				}()
			}

			wg.Wait()
		})

		It("should be thread-safe for concurrent read/write", func() {
			var wg sync.WaitGroup

			// Start a writer
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 10 {
					bundle := simulator.GenerateNewUnsBundle()
					simulator.AddUnsBundleToSimObservedState(bundle)
					time.Sleep(time.Millisecond)
				}
			}()

			// Start multiple readers
			for range 5 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range 10 {
						state := simulator.GetSimObservedState()
						Expect(state).NotTo(BeNil())
						time.Sleep(time.Millisecond)
					}
				}()
			}

			wg.Wait()
		})
	})

	Describe("HashUNSTableEntry", func() {
		It("should generate consistent hashes for same input", func() {
			topic := &tbproto.TopicInfo{
				Name:              "test.topic",
				DataContract:      "test.contract",
				Level0:            "corp",
				LocationSublevels: []string{"plant", "line"},
			}

			hash1 := topicbrowser.HashUNSTableEntry(topic)
			hash2 := topicbrowser.HashUNSTableEntry(topic)

			Expect(hash1).To(Equal(hash2))
			Expect(hash1).NotTo(BeEmpty())
		})

		It("should generate different hashes for different inputs", func() {
			topic1 := &tbproto.TopicInfo{
				Name:              "test.topic1",
				DataContract:      "test.contract",
				Level0:            "corp",
				LocationSublevels: []string{"plant", "line"},
			}

			topic2 := &tbproto.TopicInfo{
				Name:              "test.topic2",
				DataContract:      "test.contract",
				Level0:            "corp",
				LocationSublevels: []string{"plant", "line"},
			}

			hash1 := topicbrowser.HashUNSTableEntry(topic1)
			hash2 := topicbrowser.HashUNSTableEntry(topic2)

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should prevent hash collisions between different segment combinations", func() {
			// Test the specific case mentioned in comments: ["ab","c"] vs ["a","bc"]
			topic1 := &tbproto.TopicInfo{
				Name:              "test",
				DataContract:      "contract",
				Level0:            "ab",
				LocationSublevels: []string{"c"},
			}

			topic2 := &tbproto.TopicInfo{
				Name:              "test",
				DataContract:      "contract",
				Level0:            "a",
				LocationSublevels: []string{"bc"},
			}

			hash1 := topicbrowser.HashUNSTableEntry(topic1)
			hash2 := topicbrowser.HashUNSTableEntry(topic2)

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should handle virtual path correctly", func() {
			virtualPath := "virtual/path"

			topic1 := &tbproto.TopicInfo{
				Name:         "test",
				DataContract: "contract",
				Level0:       "corp",
				VirtualPath:  nil,
			}

			topic2 := &tbproto.TopicInfo{
				Name:         "test",
				DataContract: "contract",
				Level0:       "corp",
				VirtualPath:  &virtualPath,
			}

			hash1 := topicbrowser.HashUNSTableEntry(topic1)
			hash2 := topicbrowser.HashUNSTableEntry(topic2)

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should handle empty location sublevels", func() {
			topic := &tbproto.TopicInfo{
				Name:              "test",
				DataContract:      "contract",
				Level0:            "corp",
				LocationSublevels: []string{},
			}

			hash := topicbrowser.HashUNSTableEntry(topic)
			Expect(hash).NotTo(BeEmpty())
		})

		It("should return hexadecimal encoded string", func() {
			topic := &tbproto.TopicInfo{
				Name:         "test",
				DataContract: "contract",
				Level0:       "corp",
			}

			hash := topicbrowser.HashUNSTableEntry(topic)

			// Should be valid hex string
			Expect(hash).To(MatchRegexp("^[0-9a-f]+$"))
			// xxHash produces 64-bit hash, so 16 hex characters
			Expect(hash).To(HaveLen(16))
		})
	})

	Describe("Tick", func() {
		It("should increment ticker and add bundle when enabled", func() {
			initialState := simulator.GetSimObservedState()
			initialCount := len(initialState.ServiceInfo.Status.BufferSnapshot.Items)

			simulator.Tick()

			newState := simulator.GetSimObservedState()
			Expect(newState.ServiceInfo.Status.BufferSnapshot.Items).To(HaveLen(initialCount + 1))
		})

		It("should increment ticker value in generated bundles", func() {
			simulator.Tick() // ticker = 1
			bundle1 := simulator.GenerateNewUnsBundle()

			simulator.Tick() // ticker = 2
			bundle2 := simulator.GenerateNewUnsBundle()

			var unsBundle1, unsBundle2 tbproto.UnsBundle
			Expect(proto.Unmarshal(bundle1, &unsBundle1)).To(Succeed())
			Expect(proto.Unmarshal(bundle2, &unsBundle2)).To(Succeed())

			// ProducedAtMs should reflect ticker values
			event1 := unsBundle1.GetEvents().GetEntries()[0]
			event2 := unsBundle2.GetEvents().GetEntries()[0]

			Expect(event2.GetProducedAtMs()).To(BeNumerically(">=", event1.GetProducedAtMs()))
		})

		It("should work correctly with multiple ticks", func() {
			initialState := simulator.GetSimObservedState()
			initialCount := len(initialState.ServiceInfo.Status.BufferSnapshot.Items)

			numTicks := 5
			for range numTicks {
				simulator.Tick()
			}

			finalState := simulator.GetSimObservedState()
			Expect(finalState.ServiceInfo.Status.BufferSnapshot.Items).To(HaveLen(initialCount + numTicks))
		})
	})

	Describe("Integration", func() {
		It("should maintain data consistency across operations", func() {
			// Perform multiple ticks
			for range 3 {
				simulator.Tick()
			}

			state := simulator.GetSimObservedState()

			// Verify all bundles can be unmarshaled and contain valid data
			for _, buffer := range state.ServiceInfo.Status.BufferSnapshot.Items {
				var unsBundle tbproto.UnsBundle
				Expect(proto.Unmarshal(buffer.Payload, &unsBundle)).To(Succeed())

				Expect(unsBundle.GetUnsMap()).NotTo(BeNil())
				Expect(unsBundle.GetEvents()).NotTo(BeNil())
				Expect(unsBundle.GetUnsMap().GetEntries()).To(HaveLen(len(unsBundle.GetEvents().GetEntries())))

				// Verify each event has valid UnsTreeId that maps to a topic
				for _, event := range unsBundle.GetEvents().GetEntries() {
					_, exists := unsBundle.GetUnsMap().GetEntries()[event.GetUnsTreeId()]
					Expect(exists).To(BeTrue(), "Event UnsTreeId should map to existing topic")
				}
			}
		})
	})

	Describe("TestDecodeUnsBundle", func() {
		It("should decode uns bundle", func() {

			bundle := simulator.GenerateNewUnsBundle()

			var unsBundle tbproto.UnsBundle
			err := proto.Unmarshal(bundle, &unsBundle)
			Expect(err).NotTo(HaveOccurred())
			Expect(unsBundle.GetUnsMap()).NotTo(BeNil())
			Expect(unsBundle.GetEvents()).NotTo(BeNil())
		})
	})
})
