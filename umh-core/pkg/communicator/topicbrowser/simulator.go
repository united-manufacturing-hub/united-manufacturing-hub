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

package topicbrowser

import (
	"encoding/hex"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Simulator is a simulator for the topic browser service.
// The simulated observed state is used to generate the topic browser data for the status message.
// It can be enabled via the config file (Agent.Simulator) and is disabled by default.
type Simulator struct {
	simObservedState   *topicbrowserfsm.ObservedStateSnapshot
	simObservedStateMu *sync.RWMutex
	topics             map[string]*tbproto.TopicInfo
	ticker             int
	simulatorEnabled   bool
}

func NewSimulator() *Simulator {
	s := &Simulator{
		simObservedState:   &topicbrowserfsm.ObservedStateSnapshot{},
		simObservedStateMu: &sync.RWMutex{},
		ticker:             0,
		simulatorEnabled:   false,
	}

	return s
}

// InitializeSimulator initializes the simulator and adds some hardcoded topics to the simulator.
// It is called when the simulator is enabled via the config file.
func (s *Simulator) InitializeSimulator() {
	s.simulatorEnabled = true
	// add some hardcoded initial topics to the simulator and use the HashUNSTableEntry function to generate the key
	s.topics = make(map[string]*tbproto.TopicInfo)

	newTopic := &tbproto.TopicInfo{
		Name:              "uns.topic1",
		DataContract:      "uns.topic1",
		Level0:            "corpA",
		LocationSublevels: []string{"plant-1", "line-4", "pump-41"},
	}
	s.topics[HashUNSTableEntry(newTopic)] = newTopic

	newTopic = &tbproto.TopicInfo{
		Name:              "uns.topic2",
		DataContract:      "uns.topic2",
		Level0:            "corpA",
		LocationSublevels: []string{"plant-1", "line-4", "pump-42"},
	}
	s.topics[HashUNSTableEntry(newTopic)] = newTopic
}

// this function is used to generate a new uns bundle for the simulator
// it contains some hardcoded data for the topics in the simulated namespace.
func (s *Simulator) GenerateNewUnsBundle() []byte {
	// generate some new random data for the topics in the simulated namespace (s.topics)
	entries := []*tbproto.EventTableEntry{}

	for key := range s.topics {
		data := &tbproto.EventTableEntry{
			UnsTreeId: key,
			Payload: &tbproto.EventTableEntry_Ts{
				Ts: &tbproto.TimeSeriesPayload{
					ScalarType:  tbproto.ScalarType_NUMERIC,
					TimestampMs: time.Now().UnixMilli(),
					Value: &tbproto.TimeSeriesPayload_NumericValue{
						NumericValue: &wrapperspb.DoubleValue{
							Value: rand.Float64() * 100,
						},
					},
				},
			},
			ProducedAtMs: uint64(time.Now().UnixMilli()),
		}
		entries = append(entries, data)
	}

	unsBundle := &tbproto.UnsBundle{
		UnsMap: &tbproto.TopicMap{
			Entries: s.topics,
		},
		Events: &tbproto.EventTable{Entries: entries},
	}

	marshaled, err := proto.Marshal(unsBundle)
	if err != nil {
		return []byte{}
	}

	return marshaled
}

// AddUnsBundleToSimObservedState adds a new bundle to the simulated observed state.
func (s *Simulator) AddUnsBundleToSimObservedState(bundle []byte) {
	s.simObservedStateMu.Lock()
	defer s.simObservedStateMu.Unlock()

	// Create new BufferItem with sequence number
	newItem := &topicbrowserservice.BufferItem{
		Payload:     bundle,
		Timestamp:   time.Now(),
		SequenceNum: uint64(len(s.simObservedState.ServiceInfo.Status.BufferSnapshot.Items) + 1),
	}

	s.simObservedState.ServiceInfo.Status.BufferSnapshot.Items = append(s.simObservedState.ServiceInfo.Status.BufferSnapshot.Items, newItem)
	s.simObservedState.ServiceInfo.Status.BufferSnapshot.LastSequenceNum = newItem.SequenceNum

	// limit the buffer to 100 entries and delete the oldest entry if the buffer is full
	if len(s.simObservedState.ServiceInfo.Status.BufferSnapshot.Items) > 100 {
		s.simObservedState.ServiceInfo.Status.BufferSnapshot.Items = s.simObservedState.ServiceInfo.Status.BufferSnapshot.Items[1:]
	}
}

func (s *Simulator) GetSimObservedState() *topicbrowserfsm.ObservedStateSnapshot {
	s.simObservedStateMu.RLock()
	defer s.simObservedStateMu.RUnlock()

	return s.simObservedState
}

// HashUNSTableEntry generates an xxHash from the Levels and datacontract.
// This is used by the frontend to identify which topic an entry belongs to.
// We use it over full topic names to reduce the amount of data we need to send to the frontend.
//
// ✅ FIX: Uses null byte delimiters to prevent hash collisions between different segment combinations.
// For example, ["ab","c"] vs ["a","bc"] would produce different hashes instead of identical ones.
// This is a copy of the HashUNSTableEntry function in the benthos topicbrowser plugin.
func HashUNSTableEntry(info *tbproto.TopicInfo) string {
	hasher := xxhash.New()

	// Helper function to write each component followed by NUL delimiter to avoid ambiguity
	write := func(s string) {
		_, _ = hasher.Write(append([]byte(s), 0))
	}

	write(info.GetLevel0())

	// Hash all location sublevels
	for _, level := range info.GetLocationSublevels() {
		write(level)
	}

	write(info.GetDataContract())

	// Hash virtual path if it exists
	if info.VirtualPath != nil {
		write(info.GetVirtualPath())
	}

	// Hash the name (new field)
	write(info.GetName())

	return hex.EncodeToString(hasher.Sum(nil))
}

// Tick is called once per second to generate a new uns bundle and add it to the simulated observed state.
func (s *Simulator) Tick() {
	if !s.simulatorEnabled {
		return
	}

	s.ticker++
	s.AddUnsBundleToSimObservedState(s.GenerateNewUnsBundle())
}

// getSimulatorEnabled returns true if the simulator is enabled.
func (s *Simulator) GetSimulatorEnabled() bool {
	return s.simulatorEnabled
}
