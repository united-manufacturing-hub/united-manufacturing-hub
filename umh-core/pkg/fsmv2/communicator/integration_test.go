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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

func createMockAuthServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := map[string]interface{}{
			"token":     "test-jwt-token-12345",
			"expiresAt": time.Now().Add(1 * time.Hour).Unix(),
		}
		json.NewEncoder(w).Encode(token)
	}))
}

func tickFSM(ctx context.Context, worker *communicator.CommunicatorWorker, currentState fsmv2.State, desired fsmv2.DesiredState) (fsmv2.State, fsmv2.Action, error) {
	observed, err := worker.CollectObservedState(ctx)
	if err != nil {
		return nil, nil, err
	}

	snapshot := fsmv2.Snapshot{
		Observed: observed,
		Desired:  desired,
	}

	nextState, _, action := currentState.Next(snapshot)
	return nextState, action, nil
}

func TestFullFSMLifecycle(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()
	authServer := createMockAuthServer()
	defer authServer.Close()

	inboundChan := make(chan *transport.UMHMessage, 100)
	outboundChan := make(chan *transport.UMHMessage, 100)
	mockTransport := NewMockTransport()

	worker := communicator.NewCommunicatorWorker(
		"test-communicator",
		authServer.URL,
		inboundChan,
		outboundChan,
		mockTransport,
		"test-uuid",
		"test-token",
		logger,
	)

	desired, err := worker.DeriveDesiredState(nil)
	require.NoError(t, err)

	currentState := worker.GetInitialState()
	assert.IsType(t, &communicator.StoppedState{}, currentState, "Should start in StoppedState")

	nextState, action, err := tickFSM(ctx, worker, currentState, desired)
	require.NoError(t, err)
	assert.IsType(t, &communicator.TryingToAuthenticateState{}, nextState, "Should transition to TryingToAuthenticateState")
	assert.Nil(t, action, "No action on state change from Stopped")

	currentState = nextState
	nextState, action, err = tickFSM(ctx, worker, currentState, desired)
	require.NoError(t, err)
	assert.IsType(t, &communicator.TryingToAuthenticateState{}, nextState, "Should stay in TryingToAuthenticateState")
	assert.NotNil(t, action, "Should have AuthenticateAction")

	if action != nil {
		err = action.Execute(ctx)
		require.NoError(t, err, "AuthenticateAction should succeed")
	}

	currentState = nextState
	nextState, action, err = tickFSM(ctx, worker, currentState, desired)
	require.NoError(t, err)
	assert.IsType(t, &communicator.SyncingState{}, nextState, "Should transition to SyncingState after successful auth")

	currentState = nextState
	time.Sleep(100 * time.Millisecond)

	// Inject sync errors via mockTransport
	mockTransport.SetPullError(assert.AnError)

	for i := 0; i < 5; i++ {
		observed, err := worker.CollectObservedState(ctx)
		require.NoError(t, err)

		observedComm := observed.(*communicator.CommunicatorObservedState)
		observedComm.SetSyncHealthy(false)
		observedComm.SetConsecutiveErrors(i + 1)

		snapshot := fsmv2.Snapshot{
			Observed: observed,
			Desired:  desired,
		}

		nextState, _, action = currentState.Next(snapshot)
		if action != nil {
			err = action.Execute(ctx)
			assert.Error(t, err, "SyncAction should fail when errors are injected")
		}
		currentState = nextState
		time.Sleep(50 * time.Millisecond)
	}

	assert.IsType(t, &communicator.DegradedState{}, currentState, "Should transition to DegradedState after sync errors")

	// Clear sync errors via mockTransport
	mockTransport.SetPullError(nil)

	for i := 0; i < 5; i++ {
		observed, err := worker.CollectObservedState(ctx)
		require.NoError(t, err)

		observedComm := observed.(*communicator.CommunicatorObservedState)
		observedComm.SetSyncHealthy(true)
		observedComm.SetConsecutiveErrors(0)

		snapshot := fsmv2.Snapshot{
			Observed: observed,
			Desired:  desired,
		}

		nextState, _, action = currentState.Next(snapshot)
		if action != nil {
			err = action.Execute(ctx)
			assert.NoError(t, err, "SyncAction should succeed when errors are cleared")
		}
		currentState = nextState
		time.Sleep(50 * time.Millisecond)
	}

	assert.IsType(t, &communicator.SyncingState{}, currentState, "Should recover to SyncingState after errors clear")

	observed, err := worker.CollectObservedState(ctx)
	require.NoError(t, err)
	desiredWithShutdown := &communicator.CommunicatorDesiredState{}
	desiredWithShutdown.SetShutdownRequested(true)
	snapshot := fsmv2.Snapshot{
		Observed: observed,
		Desired:  desiredWithShutdown,
	}

	nextState, signal, action := currentState.Next(snapshot)
	assert.IsType(t, &communicator.StoppedState{}, nextState, "Should return to StoppedState on shutdown")
	assert.Equal(t, fsmv2.SignalNone, signal)
	assert.Nil(t, action, "Should not have action on shutdown")
}

func TestDegradedStateRecovery(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()
	authServer := createMockAuthServer()
	defer authServer.Close()

	inboundChan := make(chan *transport.UMHMessage, 100)
	outboundChan := make(chan *transport.UMHMessage, 100)
	mockTransport := NewMockTransport()

	worker := communicator.NewCommunicatorWorker(
		"test-communicator",
		authServer.URL,
		inboundChan,
		outboundChan,
		mockTransport,
		"test-uuid",
		"test-token",
		logger,
	)

	desired, err := worker.DeriveDesiredState(nil)
	require.NoError(t, err)

	var currentState fsmv2.State = &communicator.DegradedState{Worker: worker}

	observed, err := worker.CollectObservedState(ctx)
	require.NoError(t, err)

	observedComm := observed.(*communicator.CommunicatorObservedState)
	observedComm.SetSyncHealthy(true)
	observedComm.SetConsecutiveErrors(0)

	snapshot := fsmv2.Snapshot{
		Observed: observed,
		Desired:  desired,
	}

	nextState, _, _ := currentState.Next(snapshot)
	assert.IsType(t, &communicator.SyncingState{}, nextState, "Should recover to SyncingState when errors clear")
}

func TestTokenExpirationTriggersReauthentication(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	authCallCount := 0

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCallCount++

		var tokenExpiresAt time.Time
		if authCallCount == 1 {
			tokenExpiresAt = time.Now().Add(5 * time.Minute)
		} else {
			tokenExpiresAt = time.Now().Add(1 * time.Hour)
		}

		response := map[string]interface{}{
			"token":     "test-jwt-token-12345",
			"expiresAt": tokenExpiresAt.Unix(),
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer authServer.Close()

	inboundChan := make(chan *transport.UMHMessage, 100)
	outboundChan := make(chan *transport.UMHMessage, 100)
	mockTransport := NewMockTransport()

	worker := communicator.NewCommunicatorWorker(
		"test-communicator",
		authServer.URL,
		inboundChan,
		outboundChan,
		mockTransport,
		"test-uuid",
		"test-token",
		logger,
	)

	desired, err := worker.DeriveDesiredState(nil)
	require.NoError(t, err)

	currentState := worker.GetInitialState()
	assert.IsType(t, &communicator.StoppedState{}, currentState, "Should start in StoppedState")

	nextState, _, err := tickFSM(ctx, worker, currentState, desired)
	require.NoError(t, err)
	assert.IsType(t, &communicator.TryingToAuthenticateState{}, nextState, "Should transition to TryingToAuthenticateState")

	var action fsmv2.Action

	currentState = nextState
	nextState, action, err = tickFSM(ctx, worker, currentState, desired)
	require.NoError(t, err)
	assert.NotNil(t, action, "Should have AuthenticateAction")

	if action != nil {
		err = action.Execute(ctx)
		require.NoError(t, err, "First authentication should succeed")
	}

	assert.Equal(t, 1, authCallCount, "Should have called auth server once")

	observed, err := worker.CollectObservedState(ctx)
	require.NoError(t, err)
	observedComm := observed.(*communicator.CommunicatorObservedState)
	require.True(t, observedComm.IsTokenExpired(), "Token with 5-minute expiry should be considered expired (within 10-min buffer)")
	require.True(t, observedComm.IsAuthenticated(), "Should be marked as authenticated")

	currentState = nextState
	nextState, action, err = tickFSM(ctx, worker, currentState, desired)
	require.NoError(t, err)
	assert.IsType(t, &communicator.TryingToAuthenticateState{}, nextState, "Should stay in TryingToAuthenticateState due to expired token")
	assert.NotNil(t, action, "Should have AuthenticateAction for re-authentication")

	if action != nil {
		err = action.Execute(ctx)
		require.NoError(t, err, "Re-authentication should succeed")
	}

	assert.Equal(t, 2, authCallCount, "Should have called auth server twice (initial + refresh due to expired token)")

	observedAfterReauth, err := worker.CollectObservedState(ctx)
	require.NoError(t, err)
	observedCommAfterReauth := observedAfterReauth.(*communicator.CommunicatorObservedState)
	require.False(t, observedCommAfterReauth.IsTokenExpired(), "Token should not be expired after re-authentication with 1-hour token")

	currentState = nextState
	nextState, _, err = tickFSM(ctx, worker, currentState, desired)
	require.NoError(t, err)
	assert.IsType(t, &communicator.SyncingState{}, nextState, "Should transition to SyncingState with fresh token")
}
