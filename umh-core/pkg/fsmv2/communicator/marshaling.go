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

package communicator

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func ObservedStateToDocument(state *CommunicatorObservedState) (basic.Document, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	doc := basic.Document{
		"collectedAt":        state.CollectedAt.Format(time.RFC3339Nano),
		"authenticated":      state.IsAuthenticated(),
		"jwtToken":           state.GetJWTToken(),
		"tokenExpiresAt":     state.GetTokenExpiresAt().Format(time.RFC3339Nano),
		"inboundQueueSize":   float64(state.GetInboundQueueSize()),
		"outboundQueueSize":  float64(state.GetOutboundQueueSize()),
		"syncHealthy":        state.IsSyncHealthy(),
		"consecutiveErrors":  float64(state.GetConsecutiveErrors()),
	}

	return doc, nil
}

func ObservedStateFromDocument(doc basic.Document) (*CommunicatorObservedState, error) {
	if doc == nil {
		return nil, fmt.Errorf("document cannot be nil")
	}

	state := &CommunicatorObservedState{}

	if collectedAtStr, ok := doc["collectedAt"].(string); ok && collectedAtStr != "" {
		collectedAt, err := time.Parse(time.RFC3339Nano, collectedAtStr)
		if err != nil {
			return nil, fmt.Errorf("invalid collectedAt time format: %w", err)
		}
		state.CollectedAt = collectedAt
	}

	if authenticated, ok := doc["authenticated"].(bool); ok {
		state.SetAuthenticated(authenticated)
	}

	if jwtToken, ok := doc["jwtToken"].(string); ok {
		state.SetJWTToken(jwtToken)
	}

	if tokenExpiresAtStr, ok := doc["tokenExpiresAt"].(string); ok && tokenExpiresAtStr != "" {
		tokenExpiresAt, err := time.Parse(time.RFC3339Nano, tokenExpiresAtStr)
		if err != nil {
			return nil, fmt.Errorf("invalid tokenExpiresAt time format: %w", err)
		}
		state.SetTokenExpiresAt(tokenExpiresAt)
	}

	inboundQueueSize, err := parseIntFromDocument(doc, "inboundQueueSize")
	if err != nil {
		return nil, err
	}
	state.SetInboundQueueSize(inboundQueueSize)

	outboundQueueSize, err := parseIntFromDocument(doc, "outboundQueueSize")
	if err != nil {
		return nil, err
	}
	state.SetOutboundQueueSize(outboundQueueSize)

	if syncHealthy, ok := doc["syncHealthy"].(bool); ok {
		state.SetSyncHealthy(syncHealthy)
	}

	consecutiveErrors, err := parseIntFromDocument(doc, "consecutiveErrors")
	if err != nil {
		return nil, err
	}
	state.SetConsecutiveErrors(consecutiveErrors)

	return state, nil
}

func parseIntFromDocument(doc basic.Document, fieldName string) (int, error) {
	value := doc[fieldName]
	if value == nil {
		return 0, nil
	}

	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("invalid %s type: expected number, got %T", fieldName, value)
	}
}

func DesiredStateToDocument(state *CommunicatorDesiredState) (basic.Document, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	doc := basic.Document{
		"shutdownRequested": state.ShutdownRequested(),
	}

	return doc, nil
}

func DesiredStateFromDocument(doc basic.Document) (*CommunicatorDesiredState, error) {
	if doc == nil {
		return nil, fmt.Errorf("document cannot be nil")
	}

	state := &CommunicatorDesiredState{}

	if shutdownRequested, ok := doc["shutdownRequested"].(bool); ok {
		state.SetShutdownRequested(shutdownRequested)
	} else if doc["shutdownRequested"] != nil {
		return nil, fmt.Errorf("invalid shutdownRequested type: expected bool, got %T", doc["shutdownRequested"])
	}

	return state, nil
}

func IdentityToDocument(identity fsmv2.Identity) (basic.Document, error) {
	doc := basic.Document{
		"id":         identity.ID,
		"name":       identity.Name,
		"workerType": identity.WorkerType,
	}

	return doc, nil
}

func IdentityFromDocument(doc basic.Document) (fsmv2.Identity, error) {
	if doc == nil {
		return fsmv2.Identity{}, fmt.Errorf("document cannot be nil")
	}

	var identity fsmv2.Identity

	id, ok := doc["id"].(string)
	if !ok || id == "" {
		return fsmv2.Identity{}, fmt.Errorf("missing or invalid id field")
	}
	identity.ID = id

	name, ok := doc["name"].(string)
	if !ok || name == "" {
		return fsmv2.Identity{}, fmt.Errorf("missing or invalid name field")
	}
	identity.Name = name

	if workerType, ok := doc["workerType"].(string); ok {
		identity.WorkerType = workerType
	}

	return identity, nil
}
