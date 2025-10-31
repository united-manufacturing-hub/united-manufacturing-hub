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

package communicator_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func TestObservedStateToDocument(t *testing.T) {
	collectedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	tokenExpiresAt := time.Date(2025, 1, 15, 11, 30, 0, 0, time.UTC)

	state := &communicator.CommunicatorObservedState{
		CollectedAt: collectedAt,
	}
	state.SetAuthenticated(true)
	state.SetJWTToken("test-jwt-token")
	state.SetTokenExpiresAt(tokenExpiresAt)
	state.SetInboundQueueSize(10)
	state.SetOutboundQueueSize(20)
	state.SetSyncHealthy(true)
	state.SetConsecutiveErrors(3)

	doc, err := communicator.ObservedStateToDocument(state)
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.Equal(t, collectedAt.Format(time.RFC3339Nano), doc["collectedAt"])
	assert.Equal(t, true, doc["authenticated"])
	assert.Equal(t, "test-jwt-token", doc["jwtToken"])
	assert.Equal(t, tokenExpiresAt.Format(time.RFC3339Nano), doc["tokenExpiresAt"])
	assert.Equal(t, float64(10), doc["inboundQueueSize"])
	assert.Equal(t, float64(20), doc["outboundQueueSize"])
	assert.Equal(t, true, doc["syncHealthy"])
	assert.Equal(t, float64(3), doc["consecutiveErrors"])
}

func TestObservedStateFromDocument(t *testing.T) {
	collectedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	tokenExpiresAt := time.Date(2025, 1, 15, 11, 30, 0, 0, time.UTC)

	doc := basic.Document{
		"collectedAt":        collectedAt.Format(time.RFC3339Nano),
		"authenticated":      true,
		"jwtToken":           "test-jwt-token",
		"tokenExpiresAt":     tokenExpiresAt.Format(time.RFC3339Nano),
		"inboundQueueSize":   float64(10),
		"outboundQueueSize":  float64(20),
		"syncHealthy":        true,
		"consecutiveErrors":  float64(3),
	}

	state, err := communicator.ObservedStateFromDocument(doc)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, collectedAt, state.CollectedAt)
	assert.True(t, state.IsAuthenticated())
	assert.Equal(t, "test-jwt-token", state.GetJWTToken())
	assert.Equal(t, tokenExpiresAt, state.GetTokenExpiresAt())
	assert.Equal(t, 10, state.GetInboundQueueSize())
	assert.Equal(t, 20, state.GetOutboundQueueSize())
	assert.True(t, state.IsSyncHealthy())
	assert.Equal(t, 3, state.GetConsecutiveErrors())
}

func TestObservedStateRoundTrip(t *testing.T) {
	collectedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	tokenExpiresAt := time.Date(2025, 1, 15, 11, 30, 0, 0, time.UTC)

	original := &communicator.CommunicatorObservedState{
		CollectedAt: collectedAt,
	}
	original.SetAuthenticated(true)
	original.SetJWTToken("test-jwt-token")
	original.SetTokenExpiresAt(tokenExpiresAt)
	original.SetInboundQueueSize(10)
	original.SetOutboundQueueSize(20)
	original.SetSyncHealthy(true)
	original.SetConsecutiveErrors(3)

	doc, err := communicator.ObservedStateToDocument(original)
	require.NoError(t, err)

	restored, err := communicator.ObservedStateFromDocument(doc)
	require.NoError(t, err)

	assert.Equal(t, original.CollectedAt, restored.CollectedAt)
	assert.Equal(t, original.IsAuthenticated(), restored.IsAuthenticated())
	assert.Equal(t, original.GetJWTToken(), restored.GetJWTToken())
	assert.Equal(t, original.GetTokenExpiresAt(), restored.GetTokenExpiresAt())
	assert.Equal(t, original.GetInboundQueueSize(), restored.GetInboundQueueSize())
	assert.Equal(t, original.GetOutboundQueueSize(), restored.GetOutboundQueueSize())
	assert.Equal(t, original.IsSyncHealthy(), restored.IsSyncHealthy())
	assert.Equal(t, original.GetConsecutiveErrors(), restored.GetConsecutiveErrors())
}

func TestDesiredStateToDocument(t *testing.T) {
	state := &communicator.CommunicatorDesiredState{}
	state.SetShutdownRequested(true)

	doc, err := communicator.DesiredStateToDocument(state)
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.Equal(t, true, doc["shutdownRequested"])
}

func TestDesiredStateFromDocument(t *testing.T) {
	doc := basic.Document{
		"shutdownRequested": true,
	}

	state, err := communicator.DesiredStateFromDocument(doc)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.True(t, state.ShutdownRequested())
}

func TestDesiredStateRoundTrip(t *testing.T) {
	original := &communicator.CommunicatorDesiredState{}
	original.SetShutdownRequested(true)

	doc, err := communicator.DesiredStateToDocument(original)
	require.NoError(t, err)

	restored, err := communicator.DesiredStateFromDocument(doc)
	require.NoError(t, err)

	assert.Equal(t, original.ShutdownRequested(), restored.ShutdownRequested())
}

func TestIdentityToDocument(t *testing.T) {
	identity := fsmv2.Identity{
		ID:         "test-id-123",
		Name:       "Test Communicator",
		WorkerType: "communicator",
	}

	doc, err := communicator.IdentityToDocument(identity)
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.Equal(t, "test-id-123", doc["id"])
	assert.Equal(t, "Test Communicator", doc["name"])
	assert.Equal(t, "communicator", doc["workerType"])
}

func TestIdentityFromDocument(t *testing.T) {
	doc := basic.Document{
		"id":         "test-id-123",
		"name":       "Test Communicator",
		"workerType": "communicator",
	}

	identity, err := communicator.IdentityFromDocument(doc)
	require.NoError(t, err)

	assert.Equal(t, "test-id-123", identity.ID)
	assert.Equal(t, "Test Communicator", identity.Name)
	assert.Equal(t, "communicator", identity.WorkerType)
}

func TestIdentityRoundTrip(t *testing.T) {
	original := fsmv2.Identity{
		ID:         "test-id-123",
		Name:       "Test Communicator",
		WorkerType: "communicator",
	}

	doc, err := communicator.IdentityToDocument(original)
	require.NoError(t, err)

	restored, err := communicator.IdentityFromDocument(doc)
	require.NoError(t, err)

	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.Name, restored.Name)
	assert.Equal(t, original.WorkerType, restored.WorkerType)
}

func TestObservedStateFromDocument_MissingFields(t *testing.T) {
	doc := basic.Document{}

	state, err := communicator.ObservedStateFromDocument(doc)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.True(t, state.CollectedAt.IsZero())
	assert.False(t, state.IsAuthenticated())
	assert.Empty(t, state.GetJWTToken())
	assert.Equal(t, 0, state.GetInboundQueueSize())
}

func TestObservedStateFromDocument_InvalidTypes(t *testing.T) {
	doc := basic.Document{
		"collectedAt":       "invalid-time-format",
		"inboundQueueSize":  "not-a-number",
		"outboundQueueSize": "not-a-number",
		"consecutiveErrors": "not-a-number",
	}

	_, err := communicator.ObservedStateFromDocument(doc)
	assert.Error(t, err)
}

func TestDesiredStateFromDocument_InvalidType(t *testing.T) {
	doc := basic.Document{
		"shutdownRequested": "not-a-bool",
	}

	_, err := communicator.DesiredStateFromDocument(doc)
	assert.Error(t, err)
}

func TestIdentityFromDocument_MissingFields(t *testing.T) {
	doc := basic.Document{
		"id": "test-id",
	}

	_, err := communicator.IdentityFromDocument(doc)
	assert.Error(t, err)
}

func TestIdentityToDocument_EmptyFields(t *testing.T) {
	tests := []struct {
		name     string
		identity fsmv2.Identity
		wantErr  bool
	}{
		{
			name: "empty id",
			identity: fsmv2.Identity{
				ID:         "",
				Name:       "Test Name",
				WorkerType: "communicator",
			},
			wantErr: true,
		},
		{
			name: "empty name",
			identity: fsmv2.Identity{
				ID:         "test-id",
				Name:       "",
				WorkerType: "communicator",
			},
			wantErr: true,
		},
		{
			name: "both empty",
			identity: fsmv2.Identity{
				ID:         "",
				Name:       "",
				WorkerType: "communicator",
			},
			wantErr: true,
		},
		{
			name: "valid identity",
			identity: fsmv2.Identity{
				ID:         "test-id",
				Name:       "Test Name",
				WorkerType: "communicator",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := communicator.IdentityToDocument(tt.identity)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, doc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, doc)
			}
		})
	}
}
