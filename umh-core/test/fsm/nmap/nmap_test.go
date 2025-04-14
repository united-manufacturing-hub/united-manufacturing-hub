// nmap_fsm_test.go
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

//go:build test
// +build test

package nmap_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	nmap_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

var _ = Describe("Nmap FSM", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc

		mockSvc *nmap_service.MockNmapService
		inst    *nmap.NmapInstance

		mockFS filesystem.Service
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Create a mock Nmap service.
		mockSvc = nmap_service.NewMockNmapService()

		// Create a mock filesystem service.
		mockFS = filesystem.NewMockFileSystem()

		// Create a NmapConfig.
		cfg := config.NmapConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            "testing",
				DesiredFSMState: nmap.OperationalStateStopped,
			},
			NmapServiceConfig: config.NmapServiceConfig{
				Target: "127.0.0.1",
				Port:   443,
			},
		}

		// Create an instance using NewNmapInstanceWithService.
		inst = nmap.NewNmapInstanceWithService(cfg, mockSvc)
	})

	AfterEach(func() {
		cancel()
	})

	Context("When newly created", func() {
		It("should initially transition from creation to operational stopped", func() {
			// On the first reconcile, the instance should process creation steps.
			err, did := inst.Reconcile(ctx, mockFS, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			// Assuming the FSM goes to a "creating" state during the creation phase.
			Expect(inst.GetCurrentFSMState()).To(Equal("creating"))

			// On the next reconcile, the instance should complete creation and be operational in the stopped state.
			err, did = inst.Reconcile(ctx, mockFS, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopped))
		})
	})

	Context("Lifecycle transitions", func() {
		BeforeEach(func() {
			// Advance the instance to an operational state.
			inst.Reconcile(ctx, mockFS, 10)
			inst.Reconcile(ctx, mockFS, 11)
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopped))
		})

		It("stopped -> starting -> stopped", func() {
			// Set desired state to active (e.g. "monitoring_open")
			err := inst.SetDesiredFSMState(nmap.OperationalStateOpen)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger the start sequence.
			err, did := inst.Reconcile(ctx, mockFS, 12)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStarting))

			err = inst.SetDesiredFSMState(nmap.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			err, did = inst.Reconcile(ctx, mockFS, 13)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopping))

			err, did = inst.Reconcile(ctx, mockFS, 14)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopped))
		})

		It("should remain stopped when desired state is stopped", func() {
			err, did := inst.Reconcile(ctx, mockFS, 15)
			Expect(err).NotTo(HaveOccurred())
			// NOTE: did == true since s6 was reconciled
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopped))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// Advance the instance to an operational state.
			inst.Reconcile(ctx, mockFS, 20)
			inst.Reconcile(ctx, mockFS, 21)
			err := inst.SetDesiredFSMState(nmap.OperationalStateOpen)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger the start sequence: from stopped -> starting -> degraded.
			err, did := inst.Reconcile(ctx, mockFS, 22)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStarting))

			err, did = inst.Reconcile(ctx, mockFS, 23)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))
		})
		DescribeTable("testing states from -> to: ",
			func(fromState string, toState string, portFromState string, portToState string, tick uint64) {
				// Simulate that the last scan reports the port state portFromState.
				mockSvc.SetServicePortState("testing", portFromState, 10.0)

				// A reconcile call should detect the port state and trigger EventPortOpen.
				err, did := inst.Reconcile(ctx, mockFS, tick)
				Expect(err).NotTo(HaveOccurred())
				Expect(did).To(BeTrue())
				Expect(inst.GetCurrentFSMState()).To(Equal(fromState))

				// Another reconcile call should check that it remains in the state
				err, did = inst.Reconcile(ctx, mockFS, tick+1)
				Expect(err).NotTo(HaveOccurred())
				Expect(did).To(BeTrue())
				Expect(inst.GetCurrentFSMState()).To(Equal(fromState))

				// Simulate that the last scan reports the port state portToState.
				mockSvc.SetServicePortState("testing", portToState, 10.0)

				// A reconcile call should detect the port state and trigger EventPortOpen.
				err, did = inst.Reconcile(ctx, mockFS, tick+2)
				Expect(err).NotTo(HaveOccurred())
				Expect(did).To(BeTrue())
				Expect(inst.GetCurrentFSMState()).To(Equal(toState))

				// Another reconcile call should check that it remains in the state
				err, did = inst.Reconcile(ctx, mockFS, tick+3)
				Expect(err).NotTo(HaveOccurred())
				Expect(did).To(BeTrue())
				Expect(inst.GetCurrentFSMState()).To(Equal(toState))

				mockSvc.SetNmapError("testing", 10.0)

				err, did = inst.Reconcile(ctx, mockFS, tick+4)
				Expect(err).NotTo(HaveOccurred())
				Expect(did).To(BeTrue())
				Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))
			},
			// state transitions from open ->
			Entry("open -> closed",
				nmap.OperationalStateOpen, nmap.OperationalStateClosed,
				"open", "closed", uint64(24)),
			Entry("open -> filtered",
				nmap.OperationalStateOpen, nmap.OperationalStateFiltered,
				"open", "filtered", uint64(29)),
			Entry("open -> unfiltered",
				nmap.OperationalStateOpen, nmap.OperationalStateUnfiltered,
				"open", "unfiltered", uint64(31)),
			Entry("open -> open_filtered",
				nmap.OperationalStateOpen, nmap.OperationalStateOpenFiltered,
				"open", "open|filtered", uint64(36)),
			Entry("open -> closed_filtered",
				nmap.OperationalStateOpen, nmap.OperationalStateClosedFiltered,
				"open", "closed|filtered", uint64(41)),

			// state transitions from filtered ->
			Entry("filtered -> open",
				nmap.OperationalStateFiltered, nmap.OperationalStateOpen,
				"filtered", "open", uint64(46)),
			Entry("filtered -> closed",
				nmap.OperationalStateFiltered, nmap.OperationalStateClosed,
				"filtered", "closed", uint64(51)),
			Entry("filtered -> unfiltered",
				nmap.OperationalStateFiltered, nmap.OperationalStateUnfiltered,
				"filtered", "unfiltered", uint64(56)),
			Entry("filtered -> open_filtered",
				nmap.OperationalStateFiltered, nmap.OperationalStateOpenFiltered,
				"filtered", "open|filtered", uint64(61)),
			Entry("filtered -> closed_filtered",
				nmap.OperationalStateFiltered, nmap.OperationalStateClosedFiltered,
				"filtered", "closed|filtered", uint64(66)),

			// state transitions from closed ->
			Entry("closed -> open",
				nmap.OperationalStateClosed, nmap.OperationalStateOpen,
				"closed", "open", uint64(71)),
			Entry("closed -> filtered",
				nmap.OperationalStateClosed, nmap.OperationalStateFiltered,
				"closed", "filtered", uint64(76)),
			Entry("closed -> unfiltered",
				nmap.OperationalStateClosed, nmap.OperationalStateUnfiltered,
				"closed", "unfiltered", uint64(81)),
			Entry("closed -> open_filtered",
				nmap.OperationalStateClosed, nmap.OperationalStateOpenFiltered,
				"closed", "open|filtered", uint64(86)),
			Entry("closed -> closed_filtered",
				nmap.OperationalStateClosed, nmap.OperationalStateClosedFiltered,
				"closed", "closed|filtered", uint64(91)),

			// state transitions from unfiltered ->
			Entry("unfiltered -> open",
				nmap.OperationalStateUnfiltered, nmap.OperationalStateOpen,
				"unfiltered", "open", uint64(96)),
			Entry("unfiltered -> filtered",
				nmap.OperationalStateUnfiltered, nmap.OperationalStateFiltered,
				"unfiltered", "filtered", uint64(101)),
			Entry("unfiltered -> closed",
				nmap.OperationalStateUnfiltered, nmap.OperationalStateClosed,
				"unfiltered", "closed", uint64(106)),
			Entry("unfiltered -> open_filtered",
				nmap.OperationalStateUnfiltered, nmap.OperationalStateOpenFiltered,
				"unfiltered", "open|filtered", uint64(111)),
			Entry("unfiltered -> closed_filtered",
				nmap.OperationalStateUnfiltered, nmap.OperationalStateClosedFiltered,
				"unfiltered", "closed|filtered", uint64(116)),

			// state transitions from open_filtered ->
			Entry("open_filtered -> open",
				nmap.OperationalStateOpenFiltered, nmap.OperationalStateOpen,
				"open|filtered", "open", uint64(121)),
			Entry("open_filtered -> filtered",
				nmap.OperationalStateOpenFiltered, nmap.OperationalStateFiltered,
				"open|filtered", "filtered", uint64(126)),
			Entry("open_filtered -> closed",
				nmap.OperationalStateOpenFiltered, nmap.OperationalStateClosed,
				"open|filtered", "closed", uint64(131)),
			Entry("open_filtered -> unfiltered",
				nmap.OperationalStateOpenFiltered, nmap.OperationalStateUnfiltered,
				"open|filtered", "unfiltered", uint64(136)),
			Entry("open_filtered -> closed_filtered",
				nmap.OperationalStateOpenFiltered, nmap.OperationalStateClosedFiltered,
				"open|filtered", "closed|filtered", uint64(141)),

			// state transitions from closed_filtered ->
			Entry("closed_filtered -> open",
				nmap.OperationalStateClosedFiltered, nmap.OperationalStateOpen,
				"closed|filtered", "open", uint64(146)),
			Entry("closed_filtered -> filtered",
				nmap.OperationalStateClosedFiltered, nmap.OperationalStateFiltered,
				"closed|filtered", "filtered", uint64(151)),
			Entry("closed_filtered -> closed",
				nmap.OperationalStateClosedFiltered, nmap.OperationalStateClosed,
				"closed|filtered", "closed", uint64(156)),
			Entry("closed_filtered -> unfiltered",
				nmap.OperationalStateClosedFiltered, nmap.OperationalStateUnfiltered,
				"closed|filtered", "unfiltered", uint64(161)),
			Entry("closed_filtered -> open_filtered",
				nmap.OperationalStateClosedFiltered, nmap.OperationalStateOpenFiltered,
				"closed|filtered", "open|filtered", uint64(166)),
		)
		AfterEach(func() {
			if inst.IsRemoved() {
				return
			}

			err := inst.SetDesiredFSMState(nmap.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			err, did := inst.Reconcile(ctx, mockFS, 27)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopping))

			err, did = inst.Reconcile(ctx, mockFS, 28)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStopped))
		})

		It("stays degraded", func() {
			err, did := inst.Reconcile(ctx, mockFS, 25)
			Expect(err).NotTo(HaveOccurred())
			// No port event should occur; the state remains degraded.
			// NOTE: did == true since s6 got reconciled
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))
		})
	})

	Context("When monitoring is running", func() {
		BeforeEach(func() {
			// Advance the instance to an operational state.
			inst.Reconcile(ctx, mockFS, 200)
			inst.Reconcile(ctx, mockFS, 201)
			err := inst.SetDesiredFSMState(nmap.OperationalStateOpen)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger the start sequence: from stopped -> starting -> degraded.
			err, did := inst.Reconcile(ctx, mockFS, 202)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateStarting))

			err, did = inst.Reconcile(ctx, mockFS, 203)
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))
		})
		It("should permanently fail after 20 repeated ErrScanFailed", func() {
			// First, we move the instance from creation through the operational setup.
			// (Assuming instance is already in a 'degraded' state ready for monitoring.)
			err, did := inst.Reconcile(ctx, mockFS, 204)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateDegraded))

			mockSvc.SetServicePortState("testing", "open", 10.0)

			err, did = inst.Reconcile(ctx, mockFS, 205)
			Expect(err).NotTo(HaveOccurred())
			Expect(did).To(BeTrue())
			Expect(inst.GetCurrentFSMState()).To(Equal(nmap.OperationalStateOpen))

			var tick uint64 = 206
			mockSvc.ShouldErrScanFailed = true

			for i := uint64(1); i <= 20; i++ {
				inst.Reconcile(ctx, mockFS, tick)
				tick++

				// for temporary errors we expect the instance to not be removed
				if i <= 20 {
					Expect(inst.IsRemoved()).To(BeFalse(), "Iteration %d: instance should not be removed yet", i)
				} else {
					// after > 20 retries the backoffManager should remove the instance
					Expect(inst.IsRemoved()).To(BeTrue(), "Iteration %d: instance should be removed due to permanent failure", i)
					Expect(mockSvc.ForceRemoveNmapCalled).To(BeTrue(), "ForceRemoveNmap should have been called on permanent failure")
				}
			}
		})

	})
})
