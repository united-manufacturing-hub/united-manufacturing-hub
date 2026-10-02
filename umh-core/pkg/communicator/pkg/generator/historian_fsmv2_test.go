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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2historian "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/historian"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type historianObservation = fsmv2.Observation[simple.Status[fsmv2historian.TimescaleStatus]]

// historianStore serves one historian observation. A nil observation reads as
// persistence.ErrNotFound, and err replaces the read.
type historianStore struct {
	obs *historianObservation
	err error
}

func (s *historianStore) LoadObservedTyped(_ context.Context, workerType, id string, result interface{}) error {
	if workerType != fsmv2historian.Ref.WorkerType || id != config.ChildID(fsmv2historian.Ref.Name) {
		return errors.New("historianStore: unexpected ref " + workerType + "/" + id)
	}

	if s.err != nil {
		return s.err
	}

	if s.obs == nil {
		return persistence.ErrNotFound
	}

	out, ok := result.(*historianObservation)
	if !ok {
		return errors.New("historianStore: unexpected result type")
	}

	*out = *s.obs

	return nil
}

var _ = Describe("HistorianFromFSMv2", func() {
	var (
		log  *zap.SugaredLogger
		logs *observer.ObservedLogs
	)

	BeforeEach(func() {
		core, observed := observer.New(zap.WarnLevel)
		log = zap.New(core).Sugar()
		logs = observed
	})

	useStore := func(store *historianStore) {
		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynamicchildren.NewWriter(), store))
		DeferCleanup(func() { fsmv2client.SetClient(previous) })
	}

	observationAt := func(collectedAt time.Time, degraded bool) *historianObservation {
		return &historianObservation{
			CollectedAt: collectedAt,
			Status: simple.Status[fsmv2historian.TimescaleStatus]{
				Result:   fsmv2historian.TimescaleStatus{Host: "timescale", Port: 5432, Reachable: !degraded},
				Reason:   "last poll",
				Degraded: degraded,
			},
		}
	}

	It("reports an active historian for a fresh healthy observation", func() {
		useStore(&historianStore{obs: observationAt(time.Now(), false)})

		result := generator.HistorianFromFSMv2(context.Background(), log)

		Expect(result).NotTo(BeNil())
		Expect(result.Timescale.Health.Category).To(Equal(models.Active))
		Expect(result.Timescale.Health.Message).To(Equal("last poll"))
		Expect(result.Timescale.Host).To(Equal("timescale"))
		Expect(result.Timescale.Reachable).To(BeTrue())
	})

	It("reports a degraded historian for a fresh degraded observation", func() {
		useStore(&historianStore{obs: observationAt(time.Now(), true)})

		result := generator.HistorianFromFSMv2(context.Background(), log)

		Expect(result).NotTo(BeNil())
		Expect(result.Timescale.Health.Category).To(Equal(models.Degraded))
		Expect(result.Timescale.Health.Message).To(Equal("last poll"))
	})

	It("reports a stale observation as degraded, whatever its last verdict", func() {
		useStore(&historianStore{obs: observationAt(time.Now().Add(-time.Minute), false)})

		result := generator.HistorianFromFSMv2(context.Background(), log)

		Expect(result).NotTo(BeNil())
		Expect(result.Timescale.Health.Category).To(Equal(models.Degraded))
		Expect(result.Timescale.Health.Message).To(Equal("historian monitor observation is stale"))
	})

	It("omits the section when nothing is stored", func() {
		useStore(&historianStore{})

		Expect(generator.HistorianFromFSMv2(context.Background(), log)).To(BeNil())
		Expect(logs.Len()).To(Equal(0))
	})

	It("omits the section when the historian monitor was removed", func() {
		deletedAt := time.Now().Add(-time.Second)
		removed := observationAt(time.Now(), false)
		removed.DeletedAt = &deletedAt
		useStore(&historianStore{obs: removed})

		Expect(generator.HistorianFromFSMv2(context.Background(), log)).To(BeNil())
		Expect(logs.Len()).To(Equal(0))
	})

	It("omits the section and logs once when the read fails", func() {
		useStore(&historianStore{err: errors.New("store down")})

		Expect(generator.HistorianFromFSMv2(context.Background(), log)).To(BeNil())
		Expect(logs.Len()).To(Equal(1))
	})
})
