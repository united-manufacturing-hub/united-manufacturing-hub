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

package container_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
)

var _ = Describe("ContainerWorker", func() {
	var (
		worker      *container.ContainerWorker
		mockService *container_monitor.MockService
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockService = container_monitor.NewMockService()
		worker = container.NewContainerWorker("test-id", "test-container", mockService)
	})

	Describe("CollectObservedState", func() {
		Context("when container is healthy", func() {
			BeforeEach(func() {
				mockService.SetupMockForHealthyState()
			})

			It("should return observed state with Active health", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(observed).NotTo(BeNil())

				containerObserved := observed.(*container.ContainerObservedState)
				Expect(containerObserved.OverallHealth).To(Equal(models.Active))
				Expect(containerObserved.CPUHealth).To(Equal(models.Active))
				Expect(containerObserved.MemoryHealth).To(Equal(models.Active))
				Expect(containerObserved.DiskHealth).To(Equal(models.Active))
			})

			It("should populate CPU metrics", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				containerObserved := observed.(*container.ContainerObservedState)
				Expect(containerObserved.CPUUsageMCores).To(BeNumerically(">", 0))
				Expect(containerObserved.CPUCoreCount).To(BeNumerically(">", 0))
			})

			It("should populate memory metrics", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				containerObserved := observed.(*container.ContainerObservedState)
				Expect(containerObserved.MemoryUsedBytes).To(BeNumerically(">", 0))
				Expect(containerObserved.MemoryTotalBytes).To(BeNumerically(">", 0))
			})

			It("should set CollectedAt timestamp", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				containerObserved := observed.(*container.ContainerObservedState)
				Expect(containerObserved.CollectedAt).NotTo(BeZero())
			})
		})

		Context("when container is degraded", func() {
			BeforeEach(func() {
				mockService.SetupMockForDegradedState()
			})

			It("should return observed state with Degraded health", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).NotTo(HaveOccurred())

				containerObserved := observed.(*container.ContainerObservedState)
				Expect(containerObserved.OverallHealth).To(Equal(models.Degraded))
				Expect(containerObserved.CPUHealth).To(Equal(models.Degraded))
			})
		})

		Context("when GetStatus returns an error", func() {
			BeforeEach(func() {
				testError := errors.New("failed to collect metrics")
				mockService.SetupMockForError(testError)
			})

			It("should return the error", func() {
				observed, err := worker.CollectObservedState(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to collect metrics"))
				Expect(observed).To(BeNil())
			})
		})
	})

	Describe("DeriveDesiredState", func() {
		Context("with nil spec", func() {
			It("should return default desired state", func() {
				desired, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(desired).NotTo(BeNil())

				containerDesired := desired.(*container.ContainerDesiredState)
				Expect(containerDesired.ShutdownRequested()).To(BeFalse())
			})
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()
			Expect(initialState).To(BeAssignableToTypeOf(&container.StoppedState{}))
		})
	})
})
