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
	"encoding/hex"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/cespare/xxhash/v2"
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// calculateUnsTreeId computes xxhash of topic elements separated by null delimiter
func calculateUnsTreeId(level0 string, locationSublevels []string, dataContract string, virtualPath string, name string) string {
	// Build the elements in order following UNS topic convention
	elements := []string{level0}
	elements = append(elements, locationSublevels...)
	elements = append(elements, dataContract)
	if virtualPath != "" {
		elements = append(elements, virtualPath)
	}
	elements = append(elements, name)

	// Join with null delimiter and compute xxhash
	combined := strings.Join(elements, "\x00")
	hash := xxhash.Sum64String(combined)

	// Convert to hex string
	return hex.EncodeToString([]byte{
		byte(hash >> 56), byte(hash >> 48), byte(hash >> 40), byte(hash >> 32),
		byte(hash >> 24), byte(hash >> 16), byte(hash >> 8), byte(hash),
	})
}

// PopulateMockData fills the cache with realistic UNS sample data for GraphQL testing
// Uses reflection to access private fields since there's no public API for direct population
func PopulateMockData(cache *topicbrowser.Cache) {
	log := logger.For(logger.ComponentCommunicator)
	now := time.Now()

	// Mock topics following proper UNS convention: umh.v1.<location_path>.<data_contract>[.<virtual_path>].<tag_name>
	mockTopics := []struct {
		level0            string
		locationSublevels []string
		dataContract      string
		virtualPath       string
		name              string
		value             float64
		metadata          map[string]string
	}{
		{
			level0:            "acme",
			locationSublevels: []string{"cologne", "packaging", "station1"},
			dataContract:      "_historian",
			virtualPath:       "",
			name:              "temperature",
			value:             23.9,
			metadata: map[string]string{
				"unit":       "°C",
				"sensor_id":  "temp_001",
				"location":   "Packaging Station 1",
				"alarm_high": "80",
				"alarm_low":  "10",
			},
		},
		{
			level0:            "acme",
			locationSublevels: []string{"cologne", "packaging", "station1"},
			dataContract:      "_historian",
			virtualPath:       "",
			name:              "pressure",
			value:             1.19,
			metadata: map[string]string{
				"unit":       "bar",
				"sensor_id":  "press_001",
				"location":   "Packaging Station 1",
				"alarm_high": "2.0",
				"alarm_low":  "0.5",
			},
		},
		{
			level0:            "acme",
			locationSublevels: []string{"cologne", "packaging", "station1"},
			dataContract:      "_historian",
			virtualPath:       "",
			name:              "count",
			value:             1454,
			metadata: map[string]string{
				"unit":         "pieces",
				"counter_id":   "cnt_001",
				"location":     "Packaging Station 1",
				"target_daily": "2000",
			},
		},
		{
			level0:            "acme",
			locationSublevels: []string{"cologne", "assembly", "robot1"},
			dataContract:      "_historian",
			virtualPath:       "",
			name:              "cycle_time",
			value:             12.4,
			metadata: map[string]string{
				"unit":        "seconds",
				"robot_id":    "rob_001",
				"location":    "Assembly Line",
				"target_time": "12.0",
			},
		},
		{
			level0:            "acme",
			locationSublevels: []string{"cologne", "quality", "vision1"},
			dataContract:      "_historian",
			virtualPath:       "",
			name:              "defect_rate",
			value:             0.6,
			metadata: map[string]string{
				"unit":        "%",
				"camera_id":   "vis_001",
				"location":    "Quality Station",
				"target_rate": "<1.0",
			},
		},
		{
			level0:            "acme",
			locationSublevels: []string{"cologne", "energy"},
			dataContract:      "_historian",
			virtualPath:       "",
			name:              "power",
			value:             146.9,
			metadata: map[string]string{
				"unit":         "kW",
				"meter_id":     "pow_001",
				"location":     "Main Distribution",
				"contract_max": "200",
			},
		},
		{
			level0:            "acme",
			locationSublevels: []string{"cologne", "maintenance", "pump1"},
			dataContract:      "_historian",
			virtualPath:       "diagnostics",
			name:              "vibration",
			value:             2.2,
			metadata: map[string]string{
				"unit":          "mm/s",
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
		log.Errorf("Cannot access cache mutex field")
		return
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

	for i, mock := range mockTopics {
		// Calculate the proper UNS tree ID
		unsTreeId := calculateUnsTreeId(mock.level0, mock.locationSublevels, mock.dataContract, mock.virtualPath, mock.name)

		// Build full topic name following UNS convention
		topicParts := []string{"umh", "v1", mock.level0}
		topicParts = append(topicParts, mock.locationSublevels...)
		topicParts = append(topicParts, mock.dataContract)
		if mock.virtualPath != "" {
			topicParts = append(topicParts, mock.virtualPath)
		}
		topicParts = append(topicParts, mock.name)
		topicName := strings.Join(topicParts, ".")

		log.Infof("Creating mock data %d: unsTreeId=%s, topicName=%s", i, unsTreeId, topicName)

		// Create event entry
		event := &tbproto.EventTableEntry{
			UnsTreeId:    unsTreeId,
			ProducedAtMs: uint64(timestampMs),
		}
		eventMap[unsTreeId] = event

		// Create topic info with proper structure
		topicInfo := &tbproto.TopicInfo{
			Level0:            mock.level0,
			LocationSublevels: mock.locationSublevels,
			DataContract:      mock.dataContract,
			VirtualPath:       &mock.virtualPath, // Optional field
			Name:              mock.name,
			Metadata:          make(map[string]string),
		}

		// Copy metadata including unit
		for key, value := range mock.metadata {
			topicInfo.Metadata[key] = value
		}

		// Debug logging
		log.Infof("TopicInfo metadata for %s: %+v", mock.name, topicInfo.Metadata)

		// IMPORTANT: Use unsTreeId as key in unsMap.Entries, not topic name
		// This matches how the resolver expects to find entries
		unsMap.Entries[unsTreeId] = topicInfo
	}

	// Set the fields if possible
	if eventMapField.CanSet() {
		eventMapField.Set(reflect.ValueOf(eventMap))
		log.Infof("Set eventMap with %d entries", len(eventMap))
	} else {
		log.Errorf("Cannot set eventMap field")
	}

	if unsMapField.CanSet() {
		unsMapField.Set(reflect.ValueOf(unsMap))
		log.Infof("Set unsMap with %d entries", len(unsMap.Entries))
	} else {
		log.Errorf("Cannot set unsMap field")
	}

	log.Infof("Mock data population completed")
}
