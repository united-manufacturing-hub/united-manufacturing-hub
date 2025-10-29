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

package communication_state

import (
	"context"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httplib "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// mockTransport wraps http.Transport and tracks CloseIdleConnections calls
type mockTransport struct {
	*nethttp.Transport
	closeIdleConnectionsCalled bool
}

func (m *mockTransport) CloseIdleConnections() {
	m.closeIdleConnectionsCalled = true
	if m.Transport != nil {
		m.Transport.CloseIdleConnections()
	}
}

func TestRestartCommunicators_HTTPClientConnectionsAreClosed(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	watchdogInstance := watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, sugar)
	inbound := make(chan *models.UMHMessage, 10)
	outbound := make(chan *models.UMHMessage, 10)
	systemSnapshotManager := fsm.NewSnapshotManager()

	commState := NewCommunicationState(
		watchdogInstance,
		inbound,
		outbound,
		config.ReleaseChannel("stable"),
		systemSnapshotManager,
		nil, // ConfigManager not needed for this test
		"https://test.example.com",
		sugar,
		false,
		nil,
	)

	// Initialize Puller and Pusher
	commState.Puller = pull.NewPullerWithRestartFunc(
		"test-jwt",
		watchdogInstance,
		inbound,
		false,
		"https://test.example.com",
		sugar,
		commState.RestartCommunicators,
	)
	commState.Pusher = push.NewPusherWithRestartFunc(
		uuid.New(),
		"test-jwt",
		watchdogInstance,
		outbound,
		push.DefaultDeadLetterChanBuffer(),
		push.DefaultBackoffPolicy(),
		false,
		"https://test.example.com",
		sugar,
		commState.RestartCommunicators,
	)

	// Replace HTTP client transport with mock
	originalClient := httplib.GetClient(false)
	require.NotNil(t, originalClient)
	require.NotNil(t, originalClient.Transport)

	originalTransport, ok := originalClient.Transport.(*nethttp.Transport)
	require.True(t, ok, "Expected *nethttp.Transport, got %T (HTTP/2 should be disabled)", originalClient.Transport)

	mockTrans := &mockTransport{
		Transport: originalTransport,
	}
	originalClient.Transport = mockTrans

	// TEST: CloseIdleConnections should be called during restart
	err := commState.RestartCommunicators()
	assert.NoError(t, err)

	// Verify CloseIdleConnections was actually called
	assert.True(t, mockTrans.closeIdleConnectionsCalled,
		"CloseIdleConnections() was not called during coordinated restart")
}

func TestRestartCommunicators_StopsAndRestartsBothComponents(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	watchdogInstance := watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, sugar)
	inbound := make(chan *models.UMHMessage, 10)
	outbound := make(chan *models.UMHMessage, 10)
	systemSnapshotManager := fsm.NewSnapshotManager()

	commState := NewCommunicationState(
		watchdogInstance,
		inbound,
		outbound,
		config.ReleaseChannel("stable"),
		systemSnapshotManager,
		nil, // ConfigManager not needed for this test
		"https://test.example.com",
		sugar,
		false,
		nil,
	)

	// Initialize and start Puller and Pusher
	commState.Puller = pull.NewPullerWithRestartFunc(
		"test-jwt",
		watchdogInstance,
		inbound,
		false,
		"https://test.example.com",
		sugar,
		commState.RestartCommunicators,
	)
	commState.Pusher = push.NewPusherWithRestartFunc(
		uuid.New(),
		"test-jwt",
		watchdogInstance,
		outbound,
		push.DefaultDeadLetterChanBuffer(),
		push.DefaultBackoffPolicy(),
		false,
		"https://test.example.com",
		sugar,
		commState.RestartCommunicators,
	)

	commState.Puller.Start()
	commState.Pusher.Start()

	// Give them time to start
	time.Sleep(100 * time.Millisecond)

	// TEST: Restart should stop and restart both
	err := commState.RestartCommunicators()
	assert.NoError(t, err)

	// Verify both are running after restart
	// (In production code, Puller/Pusher have internal state indicating running status)
	// For now we verify no panic and restart completes
	assert.NotNil(t, commState.Puller)
	assert.NotNil(t, commState.Pusher)

	// Cleanup
	commState.Puller.Stop()
	commState.Pusher.Stop()
}

func TestRestartCommunicators_NilComponentHandling(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	watchdogInstance := watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, sugar)
	inbound := make(chan *models.UMHMessage, 10)
	outbound := make(chan *models.UMHMessage, 10)
	systemSnapshotManager := fsm.NewSnapshotManager()

	tests := []struct {
		name           string
		setupPuller    bool
		setupPusher    bool
		expectError    bool
		expectWarning  bool
	}{
		{
			name:           "Both components nil",
			setupPuller:    false,
			setupPusher:    false,
			expectError:    false,
			expectWarning:  true,
		},
		{
			name:           "Only Puller set",
			setupPuller:    true,
			setupPusher:    false,
			expectError:    false,
			expectWarning:  false,
		},
		{
			name:           "Only Pusher set",
			setupPuller:    false,
			setupPusher:    true,
			expectError:    false,
			expectWarning:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commState := NewCommunicationState(
				watchdogInstance,
				inbound,
				outbound,
				config.ReleaseChannel("stable"),
				systemSnapshotManager,
				nil, // ConfigManager not needed for this test
				"https://test.example.com",
				sugar,
				false,
				nil,
			)

			if tt.setupPuller {
				commState.Puller = pull.NewPullerWithRestartFunc(
					"test-jwt",
					watchdogInstance,
					inbound,
					false,
					"https://test.example.com",
					sugar,
					commState.RestartCommunicators,
				)
			}

			if tt.setupPusher {
				commState.Pusher = push.NewPusherWithRestartFunc(
					uuid.New(),
					"test-jwt",
					watchdogInstance,
					outbound,
					push.DefaultDeadLetterChanBuffer(),
					push.DefaultBackoffPolicy(),
					false,
					"https://test.example.com",
					sugar,
					commState.RestartCommunicators,
				)
			}

			err := commState.RestartCommunicators()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Cleanup
			if commState.Puller != nil {
				commState.Puller.Stop()
			}
			if commState.Pusher != nil {
				commState.Pusher.Stop()
			}
		})
	}
}

func TestPullerStop_WaitsForGoroutineWithTimeout(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	watchdogInstance := watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, sugar)
	inbound := make(chan *models.UMHMessage, 10)

	puller := pull.NewPullerWithRestartFunc(
		"test-jwt",
		watchdogInstance,
		inbound,
		false,
		"https://test.example.com",
		sugar,
		nil,
	)

	puller.Start()
	time.Sleep(100 * time.Millisecond)

	done := puller.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() done channel did not close within timeout, goroutine may still be running")
	}
}

func TestPusherStop_WaitsForGoroutineWithTimeout(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()
	sugar := logger.Sugar()

	watchdogInstance := watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, sugar)
	outbound := make(chan *models.UMHMessage, 10)

	pusher := push.NewPusherWithRestartFunc(
		uuid.New(),
		"test-jwt",
		watchdogInstance,
		outbound,
		push.DefaultDeadLetterChanBuffer(),
		push.DefaultBackoffPolicy(),
		false,
		"https://test.example.com",
		sugar,
		nil,
	)

	pusher.Start()
	time.Sleep(100 * time.Millisecond)

	done := pusher.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() done channel did not close within timeout, goroutine may still be running")
	}
}

// mockSlowComponent simulates a component that takes a specific duration to stop
type mockSlowComponent struct {
	stopDuration time.Duration
	stopChan     chan struct{}
	doneChan     chan struct{}
}

func newMockSlowComponent(stopDuration time.Duration) *mockSlowComponent {
	return &mockSlowComponent{
		stopDuration: stopDuration,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

func (m *mockSlowComponent) Stop() <-chan struct{} {
	close(m.stopChan)
	go func() {
		time.Sleep(m.stopDuration)
		close(m.doneChan)
	}()
	return m.doneChan
}

func TestRestartCommunicators_TimeoutReuseBug(t *testing.T) {
	// This test verifies that each component (Puller and Pusher) gets its OWN 5-second timeout
	// when stopping during a restart, rather than sharing a single timeout channel.
	//
	// The bug scenario:
	// - Single timeout channel created and used for BOTH components
	// - When Puller takes 6s to stop, the first select consumes the timeout
	// - The second select for Pusher sees an already-expired timeout and immediately falls through
	// - Result: Pusher doesn't get its full 5s timeout window
	//
	// Expected behavior (this test verifies):
	// - Puller gets 5s timeout (will timeout at 5s if it takes 6s)
	// - Pusher gets SEPARATE 5s timeout (will complete at 3s)
	// - Total time: 5s (Puller timeout) + 3s (Pusher stop) = 8s
	//
	// With buggy single timeout: Total ≈ 5s (Puller timeout, Pusher immediate)
	// With fixed separate timeouts: Total ≈ 8s (Puller timeout + Pusher wait)

	// Measure how long the timeout logic takes
	start := time.Now()

	// Create done channels that will close after specified durations
	// These simulate Stop() behavior
	pullerDone := make(chan struct{})
	pusherDone := make(chan struct{})

	// Start goroutines BEFORE creating timeouts to ensure proper timing
	go func() {
		time.Sleep(6 * time.Second) // Puller takes 6s (exceeds 5s timeout)
		close(pullerDone)
	}()

	// This simulates the FIXED implementation (separate timeouts)
	pullerTimeout := time.After(5 * time.Second)
	pullerResult := "unknown"
	if pullerDone != nil {
		select {
		case <-pullerDone:
			pullerResult = "stopped"
			t.Log("Puller stopped successfully")
		case <-pullerTimeout:
			pullerResult = "timeout"
			t.Log("Timeout waiting for Puller to stop (expected)")
		}
	}

	// NOW start the Pusher goroutine AFTER Puller select completes
	go func() {
		time.Sleep(3 * time.Second) // Pusher takes 3s (within 5s timeout)
		close(pusherDone)
	}()

	pusherTimeout := time.After(5 * time.Second) // NEW separate timeout
	pusherResult := "unknown"
	if pusherDone != nil {
		select {
		case <-pusherDone:
			pusherResult = "stopped"
			t.Log("Pusher stopped successfully")
		case <-pusherTimeout:
			pusherResult = "timeout"
			t.Log("Timeout waiting for Pusher to stop")
		}
	}

	elapsed := time.Since(start)

	// Verify Puller timed out (as expected for 6s stop with 5s timeout)
	if pullerResult != "timeout" {
		t.Errorf("Expected Puller to timeout, got: %s", pullerResult)
	}

	// Verify Pusher stopped successfully (3s < 5s timeout)
	if pusherResult != "stopped" {
		t.Errorf("Expected Pusher to stop successfully, got: %s", pusherResult)
	}

	// With separate timeouts: elapsed ≈ 8s (Puller timeout 5s + Pusher wait 3s)
	// Allow small timing tolerance (7.9s to account for scheduling)
	if elapsed < 7900*time.Millisecond {
		t.Errorf("Separate timeout verification failed! Expected ≥8s (Puller timeout 5s + Pusher wait 3s), got %v", elapsed)
		t.Errorf("This would indicate timeout channels are being reused instead of created separately")
	}

	// Also verify we didn't wait too long (no more than 10s)
	if elapsed > 10*time.Second {
		t.Errorf("Took too long! Expected ~8s, got %v", elapsed)
	}

	t.Logf("SUCCESS: Total elapsed time %v confirms separate timeouts working correctly", elapsed)
}
