package protocol

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RawMessage represents an encrypted message received from the transport layer.
// The From field is UNVERIFIED and must be validated by the authentication layer.
// This is Layer 3 of the CSE architecture - blind relay pattern from patent EP4512040A2.
type RawMessage struct {
	From      string
	Payload   []byte
	Timestamp time.Time
}

// Transport abstracts the network layer for CSE sync protocol.
// The relay is a blind proxy that forwards E2E encrypted packets between Frontend and Edge.
// This interface enables testing sync protocol logic without network dependencies.
//
// Design decisions:
// - Send/Receive are asynchronous to support real-world network latency
// - RawMessage.From is unverified because relay cannot authenticate E2E encrypted traffic
// - Timestamp enables replay attack detection at application layer
// - Context support enables graceful shutdown and request cancellation
type Transport interface {
	Send(ctx context.Context, to string, payload []byte) error
	Receive(ctx context.Context) (<-chan RawMessage, error)
	Close() error
}

// TransportNetworkError indicates network connectivity issues (connection refused, DNS failure, etc.)
type TransportNetworkError struct {
	Err error
}

func (e TransportNetworkError) Error() string {
	return fmt.Sprintf("transport network error: %v", e.Err)
}

func (e TransportNetworkError) Unwrap() error {
	return e.Err
}

// TransportOverflowError indicates message queue or buffer overflow.
// This can happen when sender is faster than receiver or network is congested.
type TransportOverflowError struct {
	Err error
}

func (e TransportOverflowError) Error() string {
	return fmt.Sprintf("transport overflow error: %v", e.Err)
}

func (e TransportOverflowError) Unwrap() error {
	return e.Err
}

// TransportConfigError indicates transport misconfiguration (invalid URL, missing credentials, etc.)
type TransportConfigError struct {
	Err error
}

func (e TransportConfigError) Error() string {
	return fmt.Sprintf("transport config error: %v", e.Err)
}

func (e TransportConfigError) Unwrap() error {
	return e.Err
}

// TransportTimeoutError indicates operation exceeded deadline.
type TransportTimeoutError struct {
	Err error
}

func (e TransportTimeoutError) Error() string {
	return fmt.Sprintf("transport timeout error: %v", e.Err)
}

func (e TransportTimeoutError) Unwrap() error {
	return e.Err
}

// TransportAuthError indicates authentication or authorization failure.
type TransportAuthError struct {
	Err error
}

func (e TransportAuthError) Error() string {
	return fmt.Sprintf("transport auth error: %v", e.Err)
}

func (e TransportAuthError) Unwrap() error {
	return e.Err
}

// MockTransport is an in-memory implementation of Transport for testing.
// It simulates network behavior without actual network I/O.
type MockTransport struct {
	mu         sync.RWMutex
	closed     bool
	msgChan    chan RawMessage
	senderUUID string
	simError   error
}

// NewMockTransport creates a new MockTransport with buffered message channel.
func NewMockTransport() *MockTransport {
	return &MockTransport{
		msgChan: make(chan RawMessage, 100),
	}
}

// Send sends a message to the specified recipient.
// In MockTransport, messages are delivered to the local Receive channel.
func (m *MockTransport) Send(ctx context.Context, to string, payload []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("transport closed")
	}

	if m.simError != nil {
		return m.simError
	}

	msg := RawMessage{
		From:      m.senderUUID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	select {
	case m.msgChan <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Receive returns a channel that delivers incoming messages.
// The channel is closed when the transport is closed.
func (m *MockTransport) Receive(ctx context.Context) (<-chan RawMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("transport closed")
	}

	return m.msgChan, nil
}

// Close closes the transport and the message channel.
func (m *MockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	close(m.msgChan)
	return nil
}

// SetSenderUUID sets the UUID used in the From field of sent messages.
func (m *MockTransport) SetSenderUUID(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.senderUUID = uuid
}

// SimulateNetworkError configures the transport to return a network error on next Send.
func (m *MockTransport) SimulateNetworkError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simError = TransportNetworkError{Err: err}
}

// SimulateOverflowError configures the transport to return an overflow error on next Send.
func (m *MockTransport) SimulateOverflowError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simError = TransportOverflowError{Err: err}
}

// SimulateTimeoutError configures the transport to return a timeout error on next Send.
func (m *MockTransport) SimulateTimeoutError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simError = TransportTimeoutError{Err: err}
}

// ClearSimulatedErrors removes any simulated error configuration.
func (m *MockTransport) ClearSimulatedErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simError = nil
}
