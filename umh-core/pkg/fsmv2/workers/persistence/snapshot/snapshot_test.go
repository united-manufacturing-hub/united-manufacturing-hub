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

package snapshot_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
)

func TestSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Persistence Snapshot Suite")
}

var _ = Describe("PersistenceConfig", func() {
	Describe("GetCompactionInterval", func() {
		It("returns the configured interval when set", func() {
			cfg := snapshot.PersistenceConfig{CompactionInterval: 2 * time.Minute}
			Expect(cfg.GetCompactionInterval()).To(Equal(2 * time.Minute))
		})

		It("returns DefaultCompactionInterval when zero", func() {
			cfg := snapshot.PersistenceConfig{}
			Expect(cfg.GetCompactionInterval()).To(Equal(snapshot.DefaultCompactionInterval))
		})
	})

	Describe("GetRetentionWindow", func() {
		It("returns the configured window when set", func() {
			cfg := snapshot.PersistenceConfig{RetentionWindow: 30 * time.Minute}
			Expect(cfg.GetRetentionWindow()).To(Equal(30 * time.Minute))
		})

		It("returns DefaultRetentionWindow when zero", func() {
			cfg := snapshot.PersistenceConfig{}
			Expect(cfg.GetRetentionWindow()).To(Equal(snapshot.DefaultRetentionWindow))
		})
	})

	Describe("GetMaintenanceInterval", func() {
		It("returns the configured interval when set", func() {
			cfg := snapshot.PersistenceConfig{MaintenanceInterval: 24 * time.Hour}
			Expect(cfg.GetMaintenanceInterval()).To(Equal(24 * time.Hour))
		})

		It("returns DefaultMaintenanceInterval when zero", func() {
			cfg := snapshot.PersistenceConfig{}
			Expect(cfg.GetMaintenanceInterval()).To(Equal(snapshot.DefaultMaintenanceInterval))
		})
	})
})

var _ = Describe("PersistenceStatus", func() {
	Describe("IsHealthy", func() {
		It("returns true when no consecutive errors", func() {
			s := snapshot.PersistenceStatus{}
			Expect(s.IsHealthy()).To(BeTrue())
		})

		It("returns false when consecutive errors exist", func() {
			s := snapshot.PersistenceStatus{ConsecutiveActionErrors: 1}
			Expect(s.IsHealthy()).To(BeFalse())
		})
	})
})
