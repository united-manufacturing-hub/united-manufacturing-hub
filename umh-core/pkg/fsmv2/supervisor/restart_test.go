// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

var _ = Describe("Collector Restart Logic", func() {
	Context("when restart is successful", func() {
		It("should increment restart count", func() {
			store := &mockStore{}
			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
				MaxRestartAttempts: 3,
			})

			err := s.RestartCollector(context.Background(), "test-worker")
			Expect(err).ToNot(HaveOccurred())
			Expect(s.GetRestartCount()).To(Equal(1))
		})
	})

	Context("when max restart attempts exceeded", func() {
		It("should panic", func() {
			store := &mockStore{}
			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
				MaxRestartAttempts: 3,
			})

			s.SetRestartCount(3)

			Expect(func() {
				_ = s.RestartCollector(context.Background(), "test-worker")
			}).To(Panic())
		})
	})

	Context("when collector recovers", func() {
		It("should reset restart counter", func() {
			store := &mockStore{
				loadSnapshot: func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
					identity := mockIdentity()
					identityDoc := basic.Document{
						"id":         identity.ID,
						"name":       identity.Name,
						"workerType": identity.WorkerType,
					}

					desiredDoc := basic.Document{}

					observedDoc := basic.Document{
						"timestamp": time.Now(),
					}

					return &storage.Snapshot{
						Identity: identityDoc,
						Desired:  desiredDoc,
						Observed: observedDoc,
					}, nil
				},
			}

			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			s.SetRestartCount(2)
			Expect(s.GetRestartCount()).To(Equal(2))

			err := s.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(s.GetRestartCount()).To(Equal(0))
		})
	})
})
