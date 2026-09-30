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

package generator_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	pullsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	pushsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// communicatorStore serves the transport, push and pull observations by their
// Go type. A nil observation reads as persistence.ErrNotFound, and err replaces
// the transport read.
type communicatorStore struct {
	transport *fsmv2.Observation[snapshot.TransportStatus]
	push      *fsmv2.Observation[pushsnapshot.PushStatus]
	pull      *fsmv2.Observation[pullsnapshot.PullStatus]
	err       error
}

func (s *communicatorStore) LoadObservedTyped(_ context.Context, workerType, id string, result interface{}) error {
	switch out := result.(type) {
	case *fsmv2.Observation[snapshot.TransportStatus]:
		if workerType != "transport" || id != "transport-001" {
			return fmt.Errorf("communicatorStore: transport read as %s/%s", workerType, id)
		}

		if s.err != nil {
			return s.err
		}

		if s.transport == nil {
			return persistence.ErrNotFound
		}

		*out = *s.transport
	case *fsmv2.Observation[pushsnapshot.PushStatus]:
		if workerType != "push" || id != "push-001" {
			return fmt.Errorf("communicatorStore: push read as %s/%s", workerType, id)
		}

		if s.push == nil {
			return persistence.ErrNotFound
		}

		*out = *s.push
	case *fsmv2.Observation[pullsnapshot.PullStatus]:
		if workerType != "pull" || id != "pull-001" {
			return fmt.Errorf("communicatorStore: pull read as %s/%s", workerType, id)
		}

		if s.pull == nil {
			return persistence.ErrNotFound
		}

		*out = *s.pull
	default:
		return fmt.Errorf("communicatorStore: unexpected %T for %s", result, workerType)
	}

	return nil
}

var _ = Describe("CommunicatorFromFSMv2", func() {
	var (
		log  *zap.SugaredLogger
		logs *observer.ObservedLogs
	)

	BeforeEach(func() {
		core, observed := observer.New(zap.WarnLevel)
		log = zap.New(core).Sugar()
		logs = observed
	})

	useStore := func(store *communicatorStore) {
		previous := fsmv2client.GetClient()

		if store == nil {
			fsmv2client.SetClient(nil)
		} else {
			fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), store))
		}

		DeferCleanup(func() { fsmv2client.SetClient(previous) })
	}

	transportAt := func(collectedAt time.Time) *fsmv2.Observation[snapshot.TransportStatus] {
		return &fsmv2.Observation[snapshot.TransportStatus]{CollectedAt: collectedAt}
	}

	expectHealth := func(result *models.Communicator, category models.HealthCategory, message string) {
		Expect(result).NotTo(BeNil())
		Expect(result.Health).NotTo(BeNil())
		Expect(result.Health.Category).To(Equal(category))
		Expect(result.Health.Message).To(Equal(message))
	}

	It("returns nil when no fsmv2 client is published", func() {
		useStore(nil)

		Expect(generator.CommunicatorFromFSMv2(context.Background(), log, 1)).To(BeNil())
	})

	It("reports unknown without logging when the transport was never stored", func() {
		useStore(&communicatorStore{})

		result := generator.CommunicatorFromFSMv2(context.Background(), log, 2)

		expectHealth(result, models.Neutral, "Communicator status unknown")
		Expect(result.State).To(BeEmpty())
		Expect(result.SubscriberCount).To(Equal(2))
		Expect(logs.Len()).To(Equal(0))
	})

	It("reports unknown and logs the error once when the transport read fails", func() {
		useStore(&communicatorStore{err: errors.New("store down")})

		result := generator.CommunicatorFromFSMv2(context.Background(), log, 3)

		expectHealth(result, models.Neutral, "Communicator status unknown")
		Expect(result.SubscriberCount).To(Equal(3))
		Expect(logs.Len()).To(Equal(1))
		entry := logs.All()[0]
		Expect(entry.Message).To(Equal("communicator status: failed to read transport observed state"))
		Expect(entry.ContextMap()).To(HaveKey("error"))
	})

	It("reports stale when the transport observation is older than its max age", func() {
		useStore(&communicatorStore{transport: transportAt(time.Now().Add(-30 * time.Second))})

		result := generator.CommunicatorFromFSMv2(context.Background(), log, 4)

		expectHealth(result, models.Degraded, "Communicator status is stale")
		Expect(result.State).To(BeEmpty())
		Expect(result.Push).To(BeNil())
		Expect(result.Pull).To(BeNil())
		Expect(result.SubscriberCount).To(Equal(4))
		Expect(logs.Len()).To(Equal(0))
	})

	It("reports stale when the transport observation has a zero CollectedAt", func() {
		useStore(&communicatorStore{transport: transportAt(time.Time{})})

		result := generator.CommunicatorFromFSMv2(context.Background(), log, 5)

		expectHealth(result, models.Degraded, "Communicator status is stale")
		Expect(result.State).To(BeEmpty())
		Expect(result.Push).To(BeNil())
		Expect(result.Pull).To(BeNil())
		Expect(result.SubscriberCount).To(Equal(5))
		Expect(logs.Len()).To(Equal(0))
	})

	It("maps a fresh transport with its children, then reports stale once the transport ages", func() {
		recent := time.Now().Add(-time.Second)
		transport := transportAt(recent)
		transport.State = "Degraded"

		push := &fsmv2.Observation[pushsnapshot.PushStatus]{CollectedAt: recent}
		push.Metrics.Worker = deps.Metrics{Counters: map[string]int64{string(deps.CounterMessagesPushed): 900}}

		pull := &fsmv2.Observation[pullsnapshot.PullStatus]{CollectedAt: recent}
		pull.Metrics.Worker = deps.Metrics{Counters: map[string]int64{string(deps.CounterMessagesPulled): 400}}

		store := &communicatorStore{transport: transport, push: push, pull: pull}
		useStore(store)

		result := generator.CommunicatorFromFSMv2(context.Background(), log, 6)

		Expect(result).NotTo(BeNil())
		Expect(result.State).To(Equal("Degraded"))
		Expect(result.Push).NotTo(BeNil())
		Expect(result.Push.Messages).To(Equal(int64(900)))
		Expect(result.Pull).NotTo(BeNil())
		Expect(result.Pull.Messages).To(Equal(int64(400)))
		Expect(result.SubscriberCount).To(Equal(6))

		store.transport.CollectedAt = time.Now().Add(-30 * time.Second)

		result = generator.CommunicatorFromFSMv2(context.Background(), log, 7)

		expectHealth(result, models.Degraded, "Communicator status is stale")
		Expect(result.State).To(BeEmpty())
		Expect(result.Push).To(BeNil())
		Expect(result.Pull).To(BeNil())
		Expect(result.SubscriberCount).To(Equal(7))
		Expect(logs.Len()).To(Equal(0))
	})
})
