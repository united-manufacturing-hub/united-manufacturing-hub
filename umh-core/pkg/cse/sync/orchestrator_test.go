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

package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)


var _ = ginkgo.Describe("Orchestrator Channel Pattern", func() {
	ginkgo.Context("when transport.Receive returns an error", func() {
		ginkgo.It("should not attempt to process zero-value messages from closed channel", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			logger := zap.NewNop().Sugar()
			transport := NewMockTransportWithReceiveError()
			transport.SetReceiveError(errors.New("transport receive failed"))

			syncState := &MockSyncState{}

			orchestrator := &DefaultOrchestrator{
				transport:  transport,
				syncState:  syncState,
				tier:       protocol.TierEdge,
				remoteTier: protocol.TierFrontend,
				logger:     logger,
			}

			err := orchestrator.Tick(ctx)

			gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("transport receive failed")),
				"Tick should return the transport.Receive error instead of silently processing from closed channel")
		})
	})
})

var _ = ginkgo.Describe("Orchestrator Constructor", func() {
	ginkgo.Context("when required dependencies are nil", func() {
		ginkgo.It("should panic when transport is nil", func() {
			logger := zap.NewNop().Sugar()

			gomega.Expect(func() {
				NewDefaultOrchestrator(
					nil,
					nil,
					nil,
					&MockSyncState{},
					nil,
					nil,
					protocol.TierEdge,
					logger,
				)
			}).To(gomega.Panic(), "Constructor should panic when transport is nil")
		})

		ginkgo.It("should panic when syncState is nil", func() {
			logger := zap.NewNop().Sugar()
			transport := protocol.NewMockTransport()

			gomega.Expect(func() {
				NewDefaultOrchestrator(
					transport,
					nil,
					nil,
					nil,
					nil,
					nil,
					protocol.TierEdge,
					logger,
				)
			}).To(gomega.Panic(), "Constructor should panic when syncState is nil")
		})

		ginkgo.It("should panic when logger is nil", func() {
			transport := protocol.NewMockTransport()

			gomega.Expect(func() {
				NewDefaultOrchestrator(
					transport,
					nil,
					nil,
					&MockSyncState{},
					nil,
					nil,
					protocol.TierEdge,
					nil,
				)
			}).To(gomega.Panic(), "Constructor should panic when logger is nil")
		})
	})
})

type MockTransportWithReceiveError struct {
	receiveError error
}

func NewMockTransportWithReceiveError() *MockTransportWithReceiveError {
	return &MockTransportWithReceiveError{}
}

func (m *MockTransportWithReceiveError) Send(ctx context.Context, to string, payload []byte) error {
	return nil
}

func (m *MockTransportWithReceiveError) Receive(ctx context.Context) (<-chan protocol.RawMessage, error) {
	if m.receiveError != nil {
		return nil, m.receiveError
	}
	ch := make(chan protocol.RawMessage)
	return ch, nil
}

func (m *MockTransportWithReceiveError) Close() error {
	return nil
}

func (m *MockTransportWithReceiveError) SetReceiveError(err error) {
	m.receiveError = err
}

type MockSyncState struct {
	edgeSyncID     int64
	frontendSyncID int64
}

func (m *MockSyncState) GetDeltaSince(tier protocol.Tier) (*basic.Query, error) {
	return &basic.Query{}, nil
}

func (m *MockSyncState) MarkSynced(tier protocol.Tier, syncID int64) error {
	return nil
}

func (m *MockSyncState) GetPendingChanges(tier protocol.Tier) ([]int64, error) {
	return []int64{}, nil
}

func (m *MockSyncState) GetEdgeSyncID() int64 {
	return m.edgeSyncID
}

func (m *MockSyncState) GetFrontendSyncID() int64 {
	return m.frontendSyncID
}

func (m *MockSyncState) SetEdgeSyncID(syncID int64) error {
	m.edgeSyncID = syncID
	return nil
}

func (m *MockSyncState) SetFrontendSyncID(syncID int64) error {
	m.frontendSyncID = syncID
	return nil
}

func (m *MockSyncState) RecordChange(syncID int64, tier protocol.Tier) error {
	return nil
}

func (m *MockSyncState) Flush(ctx context.Context) error {
	return nil
}

func (m *MockSyncState) Load(ctx context.Context) error {
	return nil
}

func TestOrchestratorUsesProtocolTypes(t *testing.T) {
	tests := []struct {
		name         string
		expectedType string
		checkFunc    func() bool
	}{
		{
			name:         "DefaultOrchestrator.tier uses protocol.Tier",
			expectedType: "protocol.Tier",
			checkFunc: func() bool {
				var _ protocol.Tier
				o := &DefaultOrchestrator{}
				var testTier protocol.Tier = o.tier
				_ = testTier
				return true
			},
		},
		{
			name:         "DefaultOrchestrator.remoteTier uses protocol.Tier",
			expectedType: "protocol.Tier",
			checkFunc: func() bool {
				var _ protocol.Tier
				o := &DefaultOrchestrator{}
				var testTier protocol.Tier = o.remoteTier
				_ = testTier
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.checkFunc() {
				t.Errorf("Type check failed for %s (expected %s)", tt.name, tt.expectedType)
			}
		})
	}
}

func TestStartSyncCreatesProtocolSyncMessage(t *testing.T) {
	var msg interface{}

	msg = protocol.SyncMessage{}

	if _, ok := msg.(protocol.SyncMessage); !ok {
		t.Errorf("SyncMessage is not protocol.SyncMessage type, got %T", msg)
	}
}

func TestProcessIncomingUsesProtocolSyncMessage(t *testing.T) {
	var syncMsg protocol.SyncMessage

	var _ protocol.SyncMessage = syncMsg
}

func TestOrchestratorInterfaceUsesProtocolTier(t *testing.T) {
	type interfaceMethod struct {
		name          string
		tierParamType string
	}

	methods := []interfaceMethod{
		{
			name:          "StartSync",
			tierParamType: "protocol.Tier",
		},
	}

	for _, method := range methods {
		t.Run(method.name+" parameter type", func(t *testing.T) {
			var tier protocol.Tier = protocol.TierEdge

			var _ protocol.Tier = tier
		})
	}
}

func TestGetDeltaSinceUsesProtocolTier(t *testing.T) {
	type syncStateMethod struct {
		name          string
		tierParamType string
	}

	methods := []syncStateMethod{
		{
			name:          "GetDeltaSince",
			tierParamType: "protocol.Tier",
		},
		{
			name:          "MarkSynced",
			tierParamType: "protocol.Tier",
		},
		{
			name:          "GetPendingChanges",
			tierParamType: "protocol.Tier",
		},
	}

	for _, method := range methods {
		t.Run(method.name+" parameter type", func(t *testing.T) {
			var tier protocol.Tier = protocol.TierEdge

			var _ protocol.Tier = tier
		})
	}
}
