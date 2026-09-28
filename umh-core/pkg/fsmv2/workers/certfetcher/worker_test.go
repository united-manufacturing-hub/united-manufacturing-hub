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

package certfetcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

// recordingCertHandler implements certificatehandler.Handler and counts the
// calls FetchAllCerts receives, so a test can tell which of two handlers the
// worker's dependencies actually hold. It implements only FetchAllCerts, the
// one method the dependencies call on it here; any other method of the
// interface panics.
type recordingCertHandler struct {
	certificatehandler.Handler

	fetchAllCertsCalls int
}

func (h *recordingCertHandler) FetchAllCerts(ctx context.Context) error {
	h.fetchAllCertsCalls++

	return nil
}

var _ = Describe("CertFetcherWorker cert handler dependency", func() {
	It("uses the handler from the dependency map over the global one", func() {
		globalHandler := &recordingCertHandler{}
		mapHandler := &recordingCertHandler{}

		register.SetGlobalDeps[*certfetcher.CertFetcherDependencies](certfetcher.WorkerTypeName,
			certfetcher.NewCertHandlerSeedDependencies(globalHandler))
		DeferCleanup(register.ClearGlobalDeps, certfetcher.WorkerTypeName)

		dependencyMap := map[string]any{}

		var mapHandlerAsHandler certificatehandler.Handler = mapHandler
		config.SetDependency(dependencyMap, certfetcher.CertHandlerKey, mapHandlerAsHandler)

		// The worker reads the handler under this literal map key; the
		// assertion fails if the key's name in the certfetcher package changes.
		Expect(dependencyMap).To(HaveKey("certfetcher.cert_handler"))

		identity := deps.Identity{ID: "map-handler-worker", WorkerType: "certfetcher"}
		built, err := factory.NewWorkerByType("certfetcher", identity, deps.NewNopFSMLogger(), nil, dependencyMap)
		Expect(err).NotTo(HaveOccurred())

		certFetcherWorker, ok := built.(*certfetcher.CertFetcherWorker)
		Expect(ok).To(BeTrue(), "expected *certfetcher.CertFetcherWorker, got %T", built)

		workerDeps := certFetcherWorker.GetDependencies()

		Expect(workerDeps.CertHandler()).To(BeIdenticalTo(mapHandler))

		err = workerDeps.FetchAllCerts(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(mapHandler.fetchAllCertsCalls).To(Equal(1))
		Expect(globalHandler.fetchAllCertsCalls).To(BeZero())
	})

	It("uses the global handler when the dependency map is nil", func() {
		globalHandler := &recordingCertHandler{}

		register.SetGlobalDeps[*certfetcher.CertFetcherDependencies](certfetcher.WorkerTypeName,
			certfetcher.NewCertHandlerSeedDependencies(globalHandler))
		DeferCleanup(register.ClearGlobalDeps, certfetcher.WorkerTypeName)

		// A nil map is the degenerate shape of the fallback. Production passes
		// a non-nil map without the key; the next spec pins that shape.
		identity := deps.Identity{ID: "global-handler-worker", WorkerType: "certfetcher"}
		built, err := factory.NewWorkerByType("certfetcher", identity, deps.NewNopFSMLogger(), nil, nil)
		Expect(err).NotTo(HaveOccurred())

		certFetcherWorker, ok := built.(*certfetcher.CertFetcherWorker)
		Expect(ok).To(BeTrue(), "expected *certfetcher.CertFetcherWorker, got %T", built)

		workerDeps := certFetcherWorker.GetDependencies()

		Expect(workerDeps.CertHandler()).To(BeIdenticalTo(globalHandler))

		err = workerDeps.FetchAllCerts(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(globalHandler.fetchAllCertsCalls).To(Equal(1))
	})

	It("uses the global handler when a non-nil dependency map lacks the key", func() {
		globalHandler := &recordingCertHandler{}

		register.SetGlobalDeps[*certfetcher.CertFetcherDependencies](certfetcher.WorkerTypeName,
			certfetcher.NewCertHandlerSeedDependencies(globalHandler))
		DeferCleanup(register.ClearGlobalDeps, certfetcher.WorkerTypeName)

		// Production shape: cmd/main.go builds the application supervisor's
		// dependency map as a non-nil empty map and nothing writes into it, so
		// the factory sees a non-nil map holding no certfetcher key.
		dependencyMap := map[string]any{}

		identity := deps.Identity{ID: "global-handler-empty-map-worker", WorkerType: "certfetcher"}
		built, err := factory.NewWorkerByType("certfetcher", identity, deps.NewNopFSMLogger(), nil, dependencyMap)
		Expect(err).NotTo(HaveOccurred())

		certFetcherWorker, ok := built.(*certfetcher.CertFetcherWorker)
		Expect(ok).To(BeTrue(), "expected *certfetcher.CertFetcherWorker, got %T", built)

		workerDeps := certFetcherWorker.GetDependencies()

		Expect(workerDeps.CertHandler()).To(BeIdenticalTo(globalHandler))

		err = workerDeps.FetchAllCerts(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(globalHandler.fetchAllCertsCalls).To(Equal(1))
	})

	It("builds from the dependency map alone when the global seed is absent", func() {
		mapHandler := &recordingCertHandler{}

		// Nothing seeds the global: the dependency map alone must supply the
		// handler, the injection shape the v2 scenarios use.
		register.ClearGlobalDeps(certfetcher.WorkerTypeName)

		dependencyMap := map[string]any{}

		var mapHandlerAsHandler certificatehandler.Handler = mapHandler
		config.SetDependency(dependencyMap, certfetcher.CertHandlerKey, mapHandlerAsHandler)

		identity := deps.Identity{ID: "map-only-handler-worker", WorkerType: "certfetcher"}
		built, err := factory.NewWorkerByType("certfetcher", identity, deps.NewNopFSMLogger(), nil, dependencyMap)
		Expect(err).NotTo(HaveOccurred())

		certFetcherWorker, ok := built.(*certfetcher.CertFetcherWorker)
		Expect(ok).To(BeTrue(), "expected *certfetcher.CertFetcherWorker, got %T", built)

		workerDeps := certFetcherWorker.GetDependencies()

		Expect(workerDeps.CertHandler()).To(BeIdenticalTo(mapHandler))

		err = workerDeps.FetchAllCerts(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(mapHandler.fetchAllCertsCalls).To(Equal(1))
	})
})
