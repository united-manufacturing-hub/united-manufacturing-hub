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
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

// TopicBrowserCacheInterface defines the interface for topic browser cache
// This interface abstracts the topic browser cache for testing and dependency injection
type TopicBrowserCacheInterface interface {
	// GetEventMap returns a map of hashed topic IDs to their latest events
	GetEventMap() map[string]*tbproto.EventTableEntry

	// GetUnsMap returns the unified namespace topic structure
	GetUnsMap() *tbproto.TopicMap
}

// SnapshotProvider provides access to system snapshots containing topic data
// This interface is for future extensibility of snapshot data sources
type SnapshotProvider interface {
	GetSnapshot() *SystemSnapshot
}

// SystemSnapshot represents the system state snapshot
// Contains information about all managed system components
type SystemSnapshot struct {
	Managers map[string]interface{}
}

// ResolverDependencies holds all external dependencies needed by the GraphQL resolver
// This struct enables dependency injection and makes testing easier
type ResolverDependencies struct {
	// SnapshotManager provides access to system state and configuration
	SnapshotManager *fsm.SnapshotManager

	// TopicBrowserCache provides real-time access to topic data and events
	TopicBrowserCache TopicBrowserCacheInterface
}
