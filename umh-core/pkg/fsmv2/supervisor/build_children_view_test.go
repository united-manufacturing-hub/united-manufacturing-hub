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

package supervisor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("ChildrenView lookup semantics vs legacy", func() {
	var (
		healthy   config.ChildInfo
		unhealthy config.ChildInfo
		stopped   config.ChildInfo
	)

	BeforeEach(func() {
		healthy = config.ChildInfo{
			Name:          "alpha",
			WorkerType:    "examplechild",
			StateName:     "Running",
			IsHealthy:     true,
			IsOperational: true,
		}
		unhealthy = config.ChildInfo{
			Name:        "beta",
			WorkerType:  "examplechild",
			StateName:   "Degraded",
			IsHealthy:   false,
			IsOperational: true,
		}
		stopped = config.ChildInfo{
			Name:       "gamma",
			WorkerType: "examplechild",
			StateName:  "Stopped",
			IsStopped:  true,
		}
	})

	Describe("Get", func() {
		It("returns the entry and true when the child is present and healthy", func() {
			view := config.NewChildrenView([]config.ChildInfo{healthy, unhealthy})

			got, ok := view.Get("alpha")
			Expect(ok).To(BeTrue())
			Expect(got.Name).To(Equal("alpha"))
			Expect(got.IsHealthy).To(BeTrue())
		})

		It("returns the entry and true when the child is present and unhealthy", func() {
			view := config.NewChildrenView([]config.ChildInfo{healthy, unhealthy})

			got, ok := view.Get("beta")
			Expect(ok).To(BeTrue())
			Expect(got.Name).To(Equal("beta"))
			Expect(got.IsHealthy).To(BeFalse())
		})

		It("returns the zero value and false when the child is absent", func() {
			view := config.NewChildrenView([]config.ChildInfo{healthy})

			got, ok := view.Get("missing")
			Expect(ok).To(BeFalse())
			Expect(got).To(Equal(config.ChildInfo{}))
		})

		It("returns a copy that cannot mutate the view contents", func() {
			view := config.NewChildrenView([]config.ChildInfo{healthy})

			got, ok := view.Get("alpha")
			Expect(ok).To(BeTrue())
			got.StateReason = "mutated"

			fresh, _ := view.Get("alpha")
			Expect(fresh.StateReason).To(BeEmpty())
		})
	})

	Describe("List", func() {
		It("returns the underlying slice in insertion order", func() {
			view := config.NewChildrenView([]config.ChildInfo{healthy, unhealthy, stopped})

			list := view.List()
			Expect(list).To(HaveLen(3))
			Expect(list[0].Name).To(Equal("alpha"))
			Expect(list[1].Name).To(Equal("beta"))
			Expect(list[2].Name).To(Equal("gamma"))
		})

		It("returns an empty slice for an empty view", func() {
			view := config.NewChildrenView(nil)
			Expect(view.List()).To(BeEmpty())
		})
	})

	Describe("aggregate predicates", func() {
		It("treats an empty view as healthy, operational, and stopped", func() {
			view := config.NewChildrenView(nil)

			Expect(view.HealthyCount).To(Equal(0))
			Expect(view.UnhealthyCount).To(Equal(0))
			Expect(view.AllHealthy).To(BeTrue())
			Expect(view.AllOperational).To(BeTrue())
			Expect(view.AllStopped).To(BeTrue())
		})

		It("classifies a mixed view by per-child phase", func() {
			view := config.NewChildrenView([]config.ChildInfo{healthy, unhealthy, stopped})

			Expect(view.HealthyCount).To(Equal(1))
			Expect(view.UnhealthyCount).To(Equal(1))
			Expect(view.AllHealthy).To(BeFalse())
			Expect(view.AllOperational).To(BeFalse())
			Expect(view.AllStopped).To(BeFalse())
		})

		It("reports AllHealthy when every child is healthy", func() {
			view := config.NewChildrenView([]config.ChildInfo{healthy, {
				Name: "alpha2", IsHealthy: true, IsOperational: true,
			}})

			Expect(view.HealthyCount).To(Equal(2))
			Expect(view.UnhealthyCount).To(Equal(0))
			Expect(view.AllHealthy).To(BeTrue())
			Expect(view.AllOperational).To(BeTrue())
			Expect(view.AllStopped).To(BeFalse())
		})

		It("reports AllOperational when degraded children remain operational", func() {
			degraded := config.ChildInfo{
				Name:          "delta",
				IsHealthy:     false,
				IsOperational: true,
			}
			view := config.NewChildrenView([]config.ChildInfo{healthy, degraded})

			Expect(view.HealthyCount).To(Equal(1))
			Expect(view.UnhealthyCount).To(Equal(1))
			Expect(view.AllHealthy).To(BeFalse())
			Expect(view.AllOperational).To(BeTrue())
		})

		It("reports AllStopped when every child is stopped", func() {
			view := config.NewChildrenView([]config.ChildInfo{stopped, {
				Name: "gamma2", IsStopped: true,
			}})

			Expect(view.HealthyCount).To(Equal(0))
			Expect(view.UnhealthyCount).To(Equal(0))
			Expect(view.AllStopped).To(BeTrue())
		})
	})

	Describe("constructor normalisation", func() {
		It("normalises a nil input slice to a non-nil empty slice", func() {
			view := config.NewChildrenView(nil)

			Expect(view.Children).NotTo(BeNil())
			Expect(view.Children).To(BeEmpty())
		})
	})
})
