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
