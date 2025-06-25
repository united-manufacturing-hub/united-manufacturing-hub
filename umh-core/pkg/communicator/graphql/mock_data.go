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

package graphql

import (
	"reflect"
	"time"
	"unsafe"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
)

// PopulateMockData fills the cache with realistic UNS sample data for GraphQL testing
// Uses reflection to access private fields since there's no public API for direct population
func PopulateMockData(cache *topicbrowser.Cache) {
	now := time.Now()

	// Mock topics with realistic UNS data
	mockData := []struct {
		unsTreeId string
		topicName string
		value     float64
		unit      string
		metadata  map[string]string
	}{
		{
			unsTreeId: "acme.cologne.packaging.station1.temperature",
			topicName: "umh.v1.acme.cologne.packaging.station1.temperature",
			value:     23.9,
			unit:      "°C",
			metadata: map[string]string{
				"sensor_id":  "temp_001",
				"location":   "Packaging Station 1",
				"alarm_high": "80",
				"alarm_low":  "10",
			},
		},
		{
			unsTreeId: "acme.cologne.packaging.station1.pressure",
			topicName: "umh.v1.acme.cologne.packaging.station1.pressure",
			value:     1.19,
			unit:      "bar",
			metadata: map[string]string{
				"sensor_id":  "press_001",
				"location":   "Packaging Station 1",
				"alarm_high": "2.0",
				"alarm_low":  "0.5",
			},
		},
		{
			unsTreeId: "acme.cologne.packaging.station1.count",
			topicName: "umh.v1.acme.cologne.packaging.station1.count",
			value:     1454,
			unit:      "pieces",
			metadata: map[string]string{
				"counter_id":   "cnt_001",
				"location":     "Packaging Station 1",
				"target_daily": "2000",
			},
		},
		{
			unsTreeId: "acme.cologne.assembly.robot1.cycle_time",
			topicName: "umh.v1.acme.cologne.assembly.robot1.cycle_time",
			value:     12.4,
			unit:      "seconds",
			metadata: map[string]string{
				"robot_id":    "rob_001",
				"location":    "Assembly Line",
				"target_time": "12.0",
			},
		},
		{
			unsTreeId: "acme.cologne.quality.vision1.defect_rate",
			topicName: "umh.v1.acme.cologne.quality.vision1.defect_rate",
			value:     0.6,
			unit:      "%",
			metadata: map[string]string{
				"camera_id":   "vis_001",
				"location":    "Quality Station",
				"target_rate": "<1.0",
			},
		},
		{
			unsTreeId: "acme.cologne.energy.main.power",
			topicName: "umh.v1.acme.cologne.energy.main.power",
			value:     146.9,
			unit:      "kW",
			metadata: map[string]string{
				"meter_id":     "pow_001",
				"location":     "Main Distribution",
				"contract_max": "200",
			},
		},
		{
			unsTreeId: "acme.cologne.maintenance.pump1.vibration",
			topicName: "umh.v1.acme.cologne.maintenance.pump1.vibration",
			value:     2.2,
			unit:      "mm/s",
			metadata: map[string]string{
				"sensor_id":     "vib_001",
				"location":      "Cooling System",
				"alarm_level":   "5.0",
				"warning_level": "3.0",
			},
		},
	}

	// Use reflection to access and populate the private cache fields
	cacheValue := reflect.ValueOf(cache).Elem()

	// Get the mutex and lock it
	muField := cacheValue.FieldByName("mu")
	if !muField.IsValid() {
		return // Cannot access mutex, skip mock data
	}

	// Get eventMap field
	eventMapField := cacheValue.FieldByName("eventMap")
	if !eventMapField.IsValid() || !eventMapField.CanSet() {
		// Try to access using unsafe
		eventMapField = reflect.NewAt(eventMapField.Type(), unsafe.Pointer(eventMapField.UnsafeAddr())).Elem()
	}

	// Get unsMap field
	unsMapField := cacheValue.FieldByName("unsMap")
	if !unsMapField.IsValid() || !unsMapField.CanSet() {
		// Try to access using unsafe
		unsMapField = reflect.NewAt(unsMapField.Type(), unsafe.Pointer(unsMapField.UnsafeAddr())).Elem()
	}

	// Create mock events and topic info
	eventMap := make(map[string]*tbproto.EventTableEntry)
	unsMap := &tbproto.TopicMap{
		Entries: make(map[string]*tbproto.TopicInfo),
	}

	timestampMs := now.UnixMilli()

	for _, mock := range mockData {
		// Create event entry - simplified to match actual protobuf structure
		event := &tbproto.EventTableEntry{
			UnsTreeId:    mock.unsTreeId,
			ProducedAtMs: uint64(timestampMs),
		}
		eventMap[mock.unsTreeId] = event

		// Create topic info
		topicInfo := &tbproto.TopicInfo{
			Name:     mock.topicName,
			Metadata: make(map[string]string),
		}
		for key, value := range mock.metadata {
			topicInfo.Metadata[key] = value
		}
		unsMap.Entries[mock.topicName] = topicInfo
	}

	// Set the fields if possible
	if eventMapField.CanSet() {
		eventMapField.Set(reflect.ValueOf(eventMap))
	}
	if unsMapField.CanSet() {
		unsMapField.Set(reflect.ValueOf(unsMap))
	}
}
