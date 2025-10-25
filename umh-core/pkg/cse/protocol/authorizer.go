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

package protocol

import (
	"context"
	"fmt"
	"sync"
)

// Authorizer provides ABAC (Attribute-Based Access Control) for CSE protocol.
// Based on patent EP4512040A2: Fine-grained permission model with subscription,
// field, and transaction-level authorization.
//
// DESIGN DECISION: Three authorization levels (not single CanAccess)
// WHY: CSE needs granular control - user may subscribe but not modify, or
// read some fields but not others. Single permission check is too coarse.
// TRADE-OFF: More complex API, but enables precise security policies.
// INSPIRED BY: Database row-level security, GraphQL field resolvers.
//
// ABAC (not RBAC): Decisions based on attributes (user ID, document type, field name)
// rather than roles. Enables flexible policies without role explosion.
//
// Fail-closed security: Deny by default, explicit allow required.
// This is Layer 3 (protocol layer) of the CSE architecture.
type Authorizer interface {
	// CanSubscribe checks if user can subscribe to collection updates.
	// Subscription enables real-time sync - user receives all document changes.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - userID: User identifier (from authentication layer)
	//   - collection: Collection name (e.g., "workers", "datapoints")
	//
	// Returns:
	//   - true if subscription allowed
	//   - false if denied
	//   - AuthorizerError if authorization check fails (e.g., policy unavailable)
	CanSubscribe(ctx context.Context, userID string, collection string) (bool, error)

	// CanReadField checks if user can read a specific field in a document.
	// Field-level authorization prevents exposure of sensitive data.
	//
	// Example: User may read "temperature" but not "operator_name" in same document.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - userID: User identifier
	//   - collection: Collection name
	//   - documentID: Document identifier
	//   - fieldName: Field name to check (e.g., "temperature", "_sync_id")
	//
	// Returns:
	//   - true if field read allowed
	//   - false if denied (field will be omitted from result)
	//   - AuthorizerError if authorization check fails
	CanReadField(ctx context.Context, userID string, collection string, documentID string, fieldName string) (bool, error)

	// CanWriteTransaction checks if user can perform write/delete operations.
	// Transaction-level authorization prevents unauthorized data modification.
	//
	// SECURITY: Even if user can subscribe and read, they may not be able to write.
	// Write permissions are separate and typically more restrictive.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - userID: User identifier
	//   - collection: Collection name
	//   - operation: Operation type ("insert", "update", "delete")
	//
	// Returns:
	//   - true if transaction allowed
	//   - false if denied (operation will be rejected)
	//   - AuthorizerError if authorization check fails
	CanWriteTransaction(ctx context.Context, userID string, collection string, operation string) (bool, error)
}

// AuthorizerError indicates authorization check failed (not denial).
// Common causes: policy unavailable, network timeout, invalid parameters.
//
// IMPORTANT: This is different from denial (false). Error means "couldn't check",
// denial means "checked and user not allowed".
type AuthorizerError struct{ Err error }

func (e AuthorizerError) Error() string {
	return fmt.Sprintf("authorization check failed: %v", e.Err)
}

func (e AuthorizerError) Unwrap() error { return e.Err }

// MockAuthorizer implements Authorizer for testing.
// Allows all operations by default. Use SetPolicy() to configure permissions.
//
// NOT SECURE - for testing only. Real implementation will integrate with
// policy engine (OPA, Casbin, etc.) or external authorization service.
type MockAuthorizer struct {
	mu             sync.RWMutex
	allowSubscribe map[string]bool // key: "userID:collection"
	allowReadField map[string]bool // key: "userID:collection:doc:field"
	allowWriteTx   map[string]bool // key: "userID:collection:operation"
	failSubscribe  bool
	failReadField  bool
	failWriteTx    bool
	simulateErr    error
}

// NewMockAuthorizer creates a MockAuthorizer that allows all operations by default.
// Use SetPolicy() methods to configure specific permissions.
func NewMockAuthorizer() *MockAuthorizer {
	return &MockAuthorizer{
		allowSubscribe: make(map[string]bool),
		allowReadField: make(map[string]bool),
		allowWriteTx:   make(map[string]bool),
	}
}

// SetSubscribePolicy configures subscription permission for user+collection.
// If not set, defaults to allow (test-friendly behavior).
func (m *MockAuthorizer) SetSubscribePolicy(userID string, collection string, allow bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", userID, collection)
	m.allowSubscribe[key] = allow
}

// SetReadFieldPolicy configures field read permission for user+collection+doc+field.
// If not set, defaults to allow (test-friendly behavior).
func (m *MockAuthorizer) SetReadFieldPolicy(userID string, collection string, documentID string, fieldName string, allow bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s:%s", userID, collection, documentID, fieldName)
	m.allowReadField[key] = allow
}

// SetWriteTransactionPolicy configures write permission for user+collection+operation.
// If not set, defaults to allow (test-friendly behavior).
func (m *MockAuthorizer) SetWriteTransactionPolicy(userID string, collection string, operation string, allow bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", userID, collection, operation)
	m.allowWriteTx[key] = allow
}

// SimulateSubscribeFailure makes next CanSubscribe() call fail with err.
func (m *MockAuthorizer) SimulateSubscribeFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failSubscribe = true
	m.simulateErr = err
}

// SimulateReadFieldFailure makes next CanReadField() call fail with err.
func (m *MockAuthorizer) SimulateReadFieldFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failReadField = true
	m.simulateErr = err
}

// SimulateWriteTransactionFailure makes next CanWriteTransaction() call fail with err.
func (m *MockAuthorizer) SimulateWriteTransactionFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failWriteTx = true
	m.simulateErr = err
}

// ClearSimulatedErrors resets all simulated error states.
func (m *MockAuthorizer) ClearSimulatedErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failSubscribe = false
	m.failReadField = false
	m.failWriteTx = false
	m.simulateErr = nil
}

// CanSubscribe implements Authorizer.CanSubscribe.
func (m *MockAuthorizer) CanSubscribe(ctx context.Context, userID string, collection string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, AuthorizerError{Err: ctx.Err()}
	default:
	}

	// Check for simulated failure
	if m.failSubscribe {
		return false, AuthorizerError{Err: m.simulateErr}
	}

	// Check policy (default allow if not set)
	key := fmt.Sprintf("%s:%s", userID, collection)
	if allow, exists := m.allowSubscribe[key]; exists {
		return allow, nil
	}

	return true, nil // Default allow for test-friendly behavior
}

// CanReadField implements Authorizer.CanReadField.
func (m *MockAuthorizer) CanReadField(ctx context.Context, userID string, collection string, documentID string, fieldName string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, AuthorizerError{Err: ctx.Err()}
	default:
	}

	// Check for simulated failure
	if m.failReadField {
		return false, AuthorizerError{Err: m.simulateErr}
	}

	// Check policy (default allow if not set)
	key := fmt.Sprintf("%s:%s:%s:%s", userID, collection, documentID, fieldName)
	if allow, exists := m.allowReadField[key]; exists {
		return allow, nil
	}

	return true, nil // Default allow for test-friendly behavior
}

// CanWriteTransaction implements Authorizer.CanWriteTransaction.
func (m *MockAuthorizer) CanWriteTransaction(ctx context.Context, userID string, collection string, operation string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, AuthorizerError{Err: ctx.Err()}
	default:
	}

	// Check for simulated failure
	if m.failWriteTx {
		return false, AuthorizerError{Err: m.simulateErr}
	}

	// Check policy (default allow if not set)
	key := fmt.Sprintf("%s:%s:%s", userID, collection, operation)
	if allow, exists := m.allowWriteTx[key]; exists {
		return allow, nil
	}

	return true, nil // Default allow for test-friendly behavior
}
