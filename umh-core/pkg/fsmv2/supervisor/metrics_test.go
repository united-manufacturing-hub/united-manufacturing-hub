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

package supervisor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Infrastructure Recovery Metrics", Label("metrics"), func() {
	Context("RecordCircuitOpen", func() {
		It("should record circuit breaker open state", func() {
			supervisorID := "test-supervisor-1"

			Expect(func() {
				supervisor.RecordCircuitOpen(supervisorID, true)
			}).NotTo(Panic())

			Expect(func() {
				supervisor.RecordCircuitOpen(supervisorID, false)
			}).NotTo(Panic())
		})
	})

	Context("RecordInfrastructureRecovery", func() {
		It("should record recovery event with duration", func() {
			supervisorID := "test-supervisor-2"
			duration := 500 * time.Millisecond

			Expect(func() {
				supervisor.RecordInfrastructureRecovery(supervisorID, duration)
			}).NotTo(Panic())
		})
	})

	Context("RecordChildHealthCheck", func() {
		It("should record child health check with status", func() {
			supervisorID := "test-supervisor-3"
			childName := "test-child"

			Expect(func() {
				supervisor.RecordChildHealthCheck(supervisorID, childName, "healthy")
			}).NotTo(Panic())

			Expect(func() {
				supervisor.RecordChildHealthCheck(supervisorID, childName, "unhealthy")
			}).NotTo(Panic())
		})
	})
})
