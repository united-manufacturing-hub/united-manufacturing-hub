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

// The worker keeps running for a few ticks after Delete, until the supervisor
// removes it. Reads in that window must already answer Deleted.
func TestGetAnswersDeletedFromTheMomentOfDelete(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	const workerType = "example"

	basicStore := memory.NewInMemoryStore()
	for _, role := range []string{storage.RoleIdentity, storage.RoleDesired, storage.RoleObserved} {
		g.Expect(basicStore.CreateCollection(ctx, workerType+"_"+role, nil)).To(gomega.Succeed())
	}

	store := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())
	client := fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), store)

	ref := dynamicchildren.Ref{WorkerType: workerType, Name: "bridge"}
	cfg := map[string]any{"greeting": "hello"}

	g.Expect(client.Upsert(ref, cfg)).To(gomega.Succeed())

	id := config.ChildID(ref.Name)
	_, err := store.SaveObserved(ctx, workerType, id, persistence.Document{
		"id":           id,
		"collected_at": time.Now().UTC(),
		"state":        "Running",
		"reachable":    true,
	})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	_, freshness, err := fsmv2client.GetFresh[deletedTestStatus](ctx, client, ref, time.Minute)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(freshness).To(gomega.Equal(fsmv2client.Fresh), "the running worker must read as Fresh before Delete")

	client.Delete(ref)

	_, err = fsmv2client.Get[deletedTestStatus](ctx, client, ref)
	g.Expect(errors.Is(err, fsmv2client.ErrWorkerDeleted)).To(gomega.BeTrue(),
		"Get right after Delete must report the worker as deleted, got %v", err)

	obs, freshness, err := fsmv2client.GetFresh[deletedTestStatus](ctx, client, ref, time.Minute)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(freshness).To(gomega.Equal(fsmv2client.Deleted),
		"GetFresh right after Delete must answer Deleted, although the worker still runs")
	g.Expect(obs.State).To(gomega.BeEmpty(), "GetFresh must not return the deleted worker's observation")

	g.Expect(client.Upsert(ref, cfg)).To(gomega.Succeed())

	_, freshness, err = fsmv2client.GetFresh[deletedTestStatus](ctx, client, ref, time.Minute)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(freshness).To(gomega.Equal(fsmv2client.Fresh), "adding the worker again must end the Deleted answer")
}
