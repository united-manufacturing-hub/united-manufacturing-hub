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

// Package protocol defines the application-layer message types for the CSE
// (Client-Side Encryption) synchronization protocol.
//
// # Architecture Overview
//
// The CSE protocol uses a three-tier architecture:
//   - Frontend tier: Management Console
//   - Relay tier: Transparent network proxy (blind to message content)
//   - Edge tier: umh-core instances
//
// Message Flow:
//
//	Frontend ⟷ Relay ⟷ Edge
//
// The relay cannot see or modify message content (end-to-end encryption).
// It only routes encrypted payloads between tiers.
//
// # Message Types
//
// The protocol supports three message types:
//
// 1. SyncMessage - Delta synchronization of data changes
// 2. HeartbeatMessage - Connection keepalive
// 3. AuthMessage - JWT authentication (future)
//
// All messages implement the ApplicationMessage interface, which provides
// common fields for routing (FromTier, ToTier) and debugging (Timestamp).
//
// # Message Processing Pipeline
//
// Sending a message:
//  1. Build ApplicationMessage (SyncMessage, HeartbeatMessage, etc.)
//  2. Serialize to JSON
//  3. Encrypt with E2E encryption (relay cannot read)
//  4. Send via Transport as RawMessage
//
// Receiving a message:
//  1. Receive RawMessage from Transport
//  2. Decrypt payload
//  3. Deserialize JSON to ApplicationMessage
//  4. Process based on message type
//
// # Security Model
//
// Messages are protected with end-to-end encryption:
//   - Relay sees only encrypted payloads
//   - Only Frontend and Edge can read message content
//   - JWT tokens authenticate tiers to the relay
//   - E2E encryption keys authenticate tier-to-tier
//
// This ensures the relay is a "blind proxy" that cannot inspect or modify
// synchronized data, only route encrypted messages.
package protocol

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

// MessageType identifies the type of application-layer message.
type MessageType string

const (
	// MessageTypeSync indicates a synchronization message containing data changes.
	MessageTypeSync MessageType = "sync"

	// MessageTypeHeartbeat indicates a keepalive message.
	MessageTypeHeartbeat MessageType = "heartbeat"

	// MessageTypeAuth indicates an authentication message.
	MessageTypeAuth MessageType = "auth"
)

// ApplicationMessage is the interface for all application-layer messages
// exchanged between Frontend and Edge tiers over the CSE protocol.
//
// These messages are serialized to JSON, encrypted with E2E encryption,
// and transmitted via the Transport interface as RawMessage payloads.
//
// The relay server cannot see or modify these messages (blind proxy).
type ApplicationMessage interface {
	GetType() MessageType
	GetFromTier() string
	GetToTier() string
	GetTimestamp() int64
}

// Tier represents a logical tier in the CSE 2-tier architecture.
// Note: The relay is NOT a tier - it's a transparent network proxy.
type Tier string

const (
	// TierFrontend represents the Management Console tier.
	TierFrontend Tier = "frontend"

	// TierEdge represents the Edge Gateway tier (umh-core instances).
	TierEdge Tier = "edge"
)

// SyncMessage carries delta synchronization data between Frontend and Edge.
// This is the primary message type for CSE's sync protocol.
//
// Message flow:
//  1. Sender builds SyncMessage with changes since last sync
//  2. Message is JSON serialized
//  3. Encrypted with E2E encryption (relay cannot read)
//  4. Sent via Transport as RawMessage
//  5. Receiver decrypts, deserializes, applies changes
type SyncMessage struct {
	// FromTier identifies the sending tier (Frontend or Edge).
	FromTier Tier `json:"fromTier"`

	// ToTier identifies the receiving tier (Frontend or Edge).
	ToTier Tier `json:"toTier"`

	// SyncID is the highest sync ID included in this message.
	// Used to track sync progress and detect missing messages.
	SyncID int64 `json:"syncId"`

	// Operations contains the actual data changes to synchronize.
	Operations []SyncOperation `json:"operations"`

	// Timestamp is Unix epoch when message was created.
	// Used for debugging and detecting clock skew.
	Timestamp int64 `json:"timestamp"`
}

// GetType returns the message type (always "sync" for SyncMessage).
func (m SyncMessage) GetType() MessageType {
	return MessageTypeSync
}

// GetFromTier returns the tier that sent this message.
func (m SyncMessage) GetFromTier() string {
	return string(m.FromTier)
}

// GetToTier returns the tier that should receive this message.
func (m SyncMessage) GetToTier() string {
	return string(m.ToTier)
}

// GetTimestamp returns when the message was created.
func (m SyncMessage) GetTimestamp() int64 {
	return m.Timestamp
}

// SyncOperation represents a single data change to synchronize.
// Each operation affects one document in one collection.
type SyncOperation struct {
	// Model is the collection name (e.g., "container_observed").
	Model string `json:"model"`

	// ID is the document's unique identifier within the collection.
	ID string `json:"id"`

	// Action is the operation type: "upsert" or "delete".
	// "upsert" creates or updates, "delete" removes.
	Action string `json:"action"`

	// Data is the complete document for upsert operations.
	// For delete operations, this may be nil or empty.
	Data basic.Document `json:"data,omitempty"`

	// SyncID is the monotonic sync identifier for this change.
	// Used to track which changes have been synchronized.
	SyncID int64 `json:"syncId"`
}

// HeartbeatMessage is sent periodically to maintain connection health.
// It carries no data but confirms the connection is alive.
type HeartbeatMessage struct {
	FromTier  Tier  `json:"fromTier"`
	ToTier    Tier  `json:"toTier"`
	Timestamp int64 `json:"timestamp"`
	Sequence  int64 `json:"sequence"` // Incrementing counter for debugging
}

// GetType returns the message type (always "heartbeat" for HeartbeatMessage).
func (m HeartbeatMessage) GetType() MessageType {
	return MessageTypeHeartbeat
}

// GetFromTier returns the tier that sent this heartbeat.
func (m HeartbeatMessage) GetFromTier() string {
	return string(m.FromTier)
}

// GetToTier returns the tier that should receive this heartbeat.
func (m HeartbeatMessage) GetToTier() string {
	return string(m.ToTier)
}

// GetTimestamp returns when the heartbeat was created.
func (m HeartbeatMessage) GetTimestamp() int64 {
	return m.Timestamp
}