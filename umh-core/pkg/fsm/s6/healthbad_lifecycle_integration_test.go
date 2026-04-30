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

package s6_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("S6 recovery directive — FSM dispatch", func() {
	var (
		ctx      context.Context
		cancel   context.CancelFunc
		mockSvc  *s6service.MockService
		registry serviceregistry.Provider
		inst     *s6fsm.S6Instance
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		mockSvc = s6service.NewMockService()
		mockSvc.MockExists = true
		mockSvc.MockHealthAction = s6service.ActionOK

		registry = serviceregistry.NewMockRegistry()
		cfg := config.S6FSMConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "test-svc",
				DesiredFSMState: s6fsm.OperationalStateRunning,
			},
		}
		var err error
		inst, err = s6fsm.NewS6InstanceWithService("/dev/null", cfg, mockSvc)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	// driveTicks runs Reconcile across `count` ticks starting at `startTick`,
	// returning early if the FSM reaches Removed. Errors are intentionally
	// swallowed so the loop can drive past tick boundaries.
	driveTicks := func(startTick uint64, count int) {
		for tick := startTick; tick < startTick+uint64(count); tick++ {
			_, _ = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, registry)
			if inst.IsRemoved() {
				return
			}
		}
	}

	It("drives the FSM to Removed when Health returns ActionRecreate from any state", func() {
		mockSvc.MockHealthAction = s6service.ActionRecreate

		driveTicks(1, 30)

		Expect(inst.IsRemoved()).To(BeTrue(),
			"FSM must reach Removed via Health() recovery dispatch within 30 ticks")
		Expect(mockSvc.ForceRemoveCalled).To(BeTrue(),
			"ForceRemove must fire on the underlying service to clear corruption")
	})

	It("does not re-fire after recovery (ForceRemove called exactly once across 50 post-recovery ticks)", func() {
		mockSvc.MockHealthAction = s6service.ActionRecreate
		driveTicks(1, 30)
		Expect(inst.IsRemoved()).To(BeTrue())
		Expect(mockSvc.ForceRemoveCallCount).To(Equal(1),
			"ForceRemove fires exactly once per corruption event")

		mockSvc.MockHealthAction = s6service.ActionOK

		for tick := uint64(31); tick < 81; tick++ {
			_, _ = inst.Reconcile(ctx, fsm.SystemSnapshot{Tick: tick}, registry)
		}

		Expect(mockSvc.ForceRemoveCallCount).To(Equal(1),
			"once Health flips back to ActionOK, no further ForceRemove calls may fire")
	})

	It("escapes from operational state when Health returns ActionRecreate mid-flight", func() {
		driveTicks(1, 30)
		Expect(inst.IsRemoved()).To(BeFalse(),
			"baseline: FSM should not be Removed under healthy mock with desired=Running")
		Expect(mockSvc.ForceRemoveCalled).To(BeFalse(),
			"baseline: no recovery should fire under healthy mock")

		mockSvc.MockHealthAction = s6service.ActionRecreate
		driveTicks(31, 30)

		Expect(inst.IsRemoved()).To(BeTrue(),
			"FSM must escape from operational state via Health() recovery dispatch")
		Expect(mockSvc.ForceRemoveCalled).To(BeTrue())
	})
})
