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

package fsmv2client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

type deletedTestStatus struct {
	Reachable bool `json:"reachable"`
}

// TestGetRefusesARemovedWorker checks what Get returns for a worker whose
// stored documents MarkDeleted has marked, for a worker that is not marked,
// and for a ref with nothing stored.
func TestGetRefusesARemovedWorker(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	const workerType = "example"

	basicStore := memory.NewInMemoryStore()
	for _, role := range []string{storage.RoleIdentity, storage.RoleDesired, storage.RoleObserved} {
		g.Expect(basicStore.CreateCollection(ctx, workerType+"_"+role, nil)).To(gomega.Succeed())
	}

	store := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())
	client := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), store)

	saveObservation := func(name string) {
		id := config.ChildID(name)
		_, err := store.SaveObserved(ctx, workerType, id, persistence.Document{
			"id":           id,
			"collected_at": time.Now().UTC(),
			"state":        "Running",
			"reachable":    true,
		})
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	live := dynamicchildren.Ref{WorkerType: workerType, Name: "live"}
	saveObservation(live.Name)

	obs, err := fsmv2client.Get[deletedTestStatus](ctx, client, live)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(obs.Status.Reachable).To(gomega.BeTrue())

	removed := dynamicchildren.Ref{WorkerType: workerType, Name: "removed"}
	saveObservation(removed.Name)
	g.Expect(store.Tombstone(ctx, workerType, config.ChildID(removed.Name), "removed")).To(gomega.Succeed())

	obs, err = fsmv2client.Get[deletedTestStatus](ctx, client, removed)
	g.Expect(errors.Is(err, fsmv2client.ErrWorkerDeleted)).To(gomega.BeTrue(), "got %v", err)
	g.Expect(obs.State).To(gomega.BeEmpty(), "Get must not return a removed worker's observation")
	g.Expect(obs.Status.Reachable).To(gomega.BeFalse())

	var deletedErr *fsmv2client.WorkerDeletedError
	g.Expect(errors.As(err, &deletedErr)).To(gomega.BeTrue())
	g.Expect(deletedErr.Ref).To(gomega.Equal(removed))
	g.Expect(deletedErr.DeletedAt.IsZero()).To(gomega.BeFalse())

	missing := dynamicchildren.Ref{WorkerType: workerType, Name: "missing"}
	_, err = fsmv2client.Get[deletedTestStatus](ctx, client, missing)
	g.Expect(errors.Is(err, fsmv2client.ErrNotFound)).To(gomega.BeTrue(), "got %v", err)
	g.Expect(errors.Is(err, fsmv2client.ErrWorkerDeleted)).To(gomega.BeFalse())
}
