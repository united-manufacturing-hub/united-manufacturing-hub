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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// GetFreshObs mirrors GetFresh but returns the FULL fsmv2.Observation[TStatus]
// so callers see the framework verdict fields (Degraded/Reason) and CollectedAt
// alongside the same Freshness classification. These specs reuse the
// stubStateReader/testStatus harness from freshness_test.go (same package).
var _ = Describe("GetFreshObs", func() {
	const maxAge = 10 * time.Second

	ref := dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: "benthos-bridge-1"}

	It("returns the full Observation, including CollectedAt, when Fresh", func() {
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(ref, map[string]any{})).To(Succeed())

		collectedAt := time.Now().Add(-1 * time.Second)
		staged := &fsmv2.Observation[testStatus]{
			CollectedAt: collectedAt,
			Status:      testStatus{V: "observed"},
		}
		client := fsmv2client.NewFSMv2Client(writer, &stubStateReader{obs: staged})

		obs, fresh, err := fsmv2client.GetFreshObs[testStatus](context.Background(), client, ref, maxAge)

		Expect(err).NotTo(HaveOccurred())
		Expect(fresh).To(Equal(fsmv2client.Fresh))
		Expect(obs.Status).To(Equal(testStatus{V: "observed"}))
		// The differentiator vs GetFresh: the full Observation carries CollectedAt
		// (and any other framework fields), which GetFresh drops.
		Expect(obs.CollectedAt).To(BeTemporally("==", collectedAt))
	})

	It("returns Unregistered and a zero Observation when the ref was never Upserted", func() {
		writer := dynamicchildren.NewWriter()
		client := fsmv2client.NewFSMv2Client(writer, &stubStateReader{})

		obs, fresh, err := fsmv2client.GetFreshObs[testStatus](context.Background(), client, ref, maxAge)

		Expect(err).NotTo(HaveOccurred())
		Expect(fresh).To(Equal(fsmv2client.Unregistered))
		Expect(obs).To(Equal(fsmv2.Observation[testStatus]{}))
	})

	It("returns NeverObserved when the ref is Upserted but the store returns ErrNotFound", func() {
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(ref, map[string]any{})).To(Succeed())
		client := fsmv2client.NewFSMv2Client(writer, &stubStateReader{err: persistence.ErrNotFound})

		obs, fresh, err := fsmv2client.GetFreshObs[testStatus](context.Background(), client, ref, maxAge)

		Expect(err).NotTo(HaveOccurred())
		Expect(fresh).To(Equal(fsmv2client.NeverObserved))
		Expect(obs).To(Equal(fsmv2.Observation[testStatus]{}))
	})

	It("returns Stale when CollectedAt is older than maxAge", func() {
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(ref, map[string]any{})).To(Succeed())

		collectedAt := time.Now().Add(-3 * maxAge)
		staged := &fsmv2.Observation[testStatus]{
			CollectedAt: collectedAt,
			Status:      testStatus{V: "observed"},
		}
		client := fsmv2client.NewFSMv2Client(writer, &stubStateReader{obs: staged})

		obs, fresh, err := fsmv2client.GetFreshObs[testStatus](context.Background(), client, ref, maxAge)

		Expect(err).NotTo(HaveOccurred())
		Expect(fresh).To(Equal(fsmv2client.Stale))
		Expect(obs.Status).To(Equal(testStatus{V: "observed"}))
		// Stale, like Fresh, returns the full Observation (not the zero value).
		Expect(obs.CollectedAt).To(BeTemporally("==", collectedAt))
	})

	It("returns Unknown and the error verbatim on a non-ErrNotObserved store read error", func() {
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(ref, map[string]any{})).To(Succeed())

		readErr := errors.New("store read boom")
		client := fsmv2client.NewFSMv2Client(writer, &stubStateReader{err: readErr})

		obs, fresh, err := fsmv2client.GetFreshObs[testStatus](context.Background(), client, ref, maxAge)

		Expect(err).To(MatchError(readErr))
		Expect(fresh).To(Equal(fsmv2client.Unknown))
		Expect(obs).To(Equal(fsmv2.Observation[testStatus]{}))
	})
})
