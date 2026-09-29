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

package health_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/health"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("FreshnessChecker", func() {
	Context("when observation data is fresh", func() {
		It("should return true", func() {
			checker := health.NewFreshnessChecker(
				10*time.Second,
				20*time.Second,
				"test",
				deps.NewNopFSMLogger(),
			)

			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: &supervisor.TestObservedState{
					ID:          "test-worker",
					CollectedAt: time.Now(),
					Desired:     &supervisor.TestDesiredState{},
				},
				Desired: &supervisor.TestDesiredState{},
			}

			Expect(checker.Check(snapshot)).To(BeTrue())
		})
	})

	Context("when observation data is stale", func() {
		It("should return false", func() {
			checker := health.NewFreshnessChecker(
				10*time.Second,
				20*time.Second,
				"test",
				deps.NewNopFSMLogger(),
			)

			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: &supervisor.TestObservedState{
					ID:          "test-worker",
					CollectedAt: time.Now().Add(-15 * time.Second),
					Desired:     &supervisor.TestDesiredState{},
				},
				Desired: &supervisor.TestDesiredState{},
			}

			Expect(checker.Check(snapshot)).To(BeFalse())
		})
	})

	Context("when observation data has timed out", func() {
		It("should detect timeout", func() {
			checker := health.NewFreshnessChecker(
				10*time.Second,
				20*time.Second,
				"test",
				deps.NewNopFSMLogger(),
			)

			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: &supervisor.TestObservedState{
					ID:          "test-worker",
					CollectedAt: time.Now().Add(-25 * time.Second),
					Desired:     &supervisor.TestDesiredState{},
				},
				Desired: &supervisor.TestDesiredState{},
			}

			Expect(checker.IsTimeout(snapshot)).To(BeTrue())
		})
	})

	// An observation whose age cannot be read is not evidence of freshness.
	// Treating it as fresh disabled all four defense layers for that worker:
	// the FSM kept transitioning on data of unknown age, the collector was
	// never restarted, and shutdown was never requested.
	Context("when the observation's age cannot be determined", func() {
		newChecker := func() *health.FreshnessChecker {
			return health.NewFreshnessChecker(
				10*time.Second,
				20*time.Second,
				"test",
				deps.NewNopFSMLogger(),
			)
		}

		It("treats an unrecognised observed type as stale", func() {
			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: struct{ Unrelated string }{Unrelated: "no timestamp here"},
				Desired:  &supervisor.TestDesiredState{},
			}

			Expect(newChecker().Check(snapshot)).To(BeFalse())
			Expect(newChecker().IsTimeout(snapshot)).To(BeTrue())
		})

		It("treats a document without collected_at as stale", func() {
			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: persistence.Document{"id": "test-worker"},
				Desired:  &supervisor.TestDesiredState{},
			}

			Expect(newChecker().Check(snapshot)).To(BeFalse())
			Expect(newChecker().IsTimeout(snapshot)).To(BeTrue())
		})

		It("treats an unparseable collected_at as stale", func() {
			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: persistence.Document{"collected_at": "not a timestamp"},
				Desired:  &supervisor.TestDesiredState{},
			}

			Expect(newChecker().Check(snapshot)).To(BeFalse())
			Expect(newChecker().IsTimeout(snapshot)).To(BeTrue())
		})

		It("still reads a document that carries collected_at", func() {
			snapshot := &fsmv2.Snapshot{
				Identity: supervisor.TestIdentity(),
				Observed: persistence.Document{
					"collected_at": time.Now().Format(time.RFC3339Nano),
				},
				Desired: &supervisor.TestDesiredState{},
			}

			Expect(newChecker().Check(snapshot)).To(BeTrue())
			Expect(newChecker().IsTimeout(snapshot)).To(BeFalse())
		})
	})
})
