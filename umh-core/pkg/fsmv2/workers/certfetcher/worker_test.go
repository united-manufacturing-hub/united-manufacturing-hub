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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

// recordingCertHandler's embedded Handler is nil, so any method but
// FetchAllCerts panics.
type recordingCertHandler struct {
	certificatehandler.Handler

	fetchAllCertsCalls int
}

func (h *recordingCertHandler) FetchAllCerts(ctx context.Context) error {
	h.fetchAllCertsCalls++

	return nil
}

var _ = Describe("CertFetcherWorker cert handler dependency", func() {
	It("returns an error naming the key when the dependency map holds no cert handler", func() {
		identity := deps.Identity{ID: "missing-handler-worker", WorkerType: "certfetcher"}

		worker, err := certfetcher.NewCertFetcherWorker(identity, deps.NewNopFSMLogger(), nil, map[string]any{})
		Expect(err).To(MatchError(`certfetcher: no cert handler under "certfetcher.cert_handler" in the dependency map`))
		Expect(worker).To(BeNil())

		worker, err = certfetcher.NewCertFetcherWorker(identity, deps.NewNopFSMLogger(), nil, nil)
		Expect(err).To(MatchError(`certfetcher: no cert handler under "certfetcher.cert_handler" in the dependency map`))
		Expect(worker).To(BeNil())
	})

	It("uses the handler from the dependency map", func() {
		mapHandler := &recordingCertHandler{}

		dependencyMap := map[string]any{}

		// Declared as the interface: a *recordingCertHandler argument does not
		// match the key's type, so SetDependency would not compile.
		var mapHandlerAsHandler certificatehandler.Handler = mapHandler
		config.SetDependency(dependencyMap, certfetcher.CertHandlerKey, mapHandlerAsHandler)

		identity := deps.Identity{ID: "map-handler-worker", WorkerType: "certfetcher"}
		built, err := certfetcher.NewCertFetcherWorker(identity, deps.NewNopFSMLogger(), nil, dependencyMap)
		Expect(err).NotTo(HaveOccurred())

		workerDeps := built.GetDependencies()

		Expect(workerDeps.CertHandler()).To(BeIdenticalTo(mapHandler))

		err = workerDeps.FetchAllCerts(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(mapHandler.fetchAllCertsCalls).To(Equal(1))
	})
})
