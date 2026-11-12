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

package storage_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// This spec names a worker's status type, so it stays in umh-core. It keeps
// the container names it had in triangular_test.go.
var _ = Describe("TriangularStore", func() {
	Describe("deriveWorkerType", func() {
		It("should derive application from Observation[ApplicationStatus]", func() {
			workerType, err := storage.DeriveWorkerType[fsmv2.Observation[snapshot.ApplicationStatus]]()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerType).To(Equal("application"))
		})
	})
})
