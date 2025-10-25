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
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

var _ = Describe("Serialization", func() {
	Describe("toDocument", func() {
		It("converts basic struct to Document", func() {
			type TestStruct struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			}

			input := TestStruct{
				Name:  "test",
				Count: 42,
			}

			doc, err := supervisor.ToDocument(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).To(HaveKeyWithValue("name", "test"))
			Expect(doc).To(HaveKeyWithValue("count", 42))
		})

		It("converts struct with nested structs", func() {
			type Inner struct {
				Value string `json:"value"`
			}
			type Outer struct {
				Field string `json:"field"`
				Nest  Inner  `json:"nest"`
			}

			input := Outer{
				Field: "outer",
				Nest: Inner{
					Value: "inner",
				},
			}

			doc, err := supervisor.ToDocument(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).To(HaveKeyWithValue("field", "outer"))

			nest, ok := doc["nest"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(nest).To(HaveKeyWithValue("value", "inner"))
		})

		It("converts struct with time.Time fields", func() {
			type TestStruct struct {
				Timestamp time.Time `json:"timestamp"`
			}

			now := time.Now().UTC()
			input := TestStruct{
				Timestamp: now,
			}

			doc, err := supervisor.ToDocument(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).To(HaveKey("timestamp"))

			ts, ok := doc["timestamp"].(time.Time)
			Expect(ok).To(BeTrue())
			Expect(ts.Unix()).To(Equal(now.Unix()))
		})
	})

	Describe("fromDocument", func() {
		It("converts Document to typed struct", func() {
			type TestStruct struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			}

			doc := basic.Document{
				"name":  "test",
				"count": 42,
			}

			targetType := reflect.TypeOf(TestStruct{})
			result, err := supervisor.FromDocument(doc, targetType)
			Expect(err).NotTo(HaveOccurred())

			output, ok := result.(TestStruct)
			Expect(ok).To(BeTrue())
			Expect(output.Name).To(Equal("test"))
			Expect(output.Count).To(Equal(42))
		})

		It("converts Document with nested maps to nested structs", func() {
			type Inner struct {
				Value string `json:"value"`
			}
			type Outer struct {
				Field string `json:"field"`
				Nest  Inner  `json:"nest"`
			}

			doc := basic.Document{
				"field": "outer",
				"nest": map[string]interface{}{
					"value": "inner",
				},
			}

			targetType := reflect.TypeOf(Outer{})
			result, err := supervisor.FromDocument(doc, targetType)
			Expect(err).NotTo(HaveOccurred())

			output, ok := result.(Outer)
			Expect(ok).To(BeTrue())
			Expect(output.Field).To(Equal("outer"))
			Expect(output.Nest.Value).To(Equal("inner"))
		})

		It("converts Document with time.Time fields", func() {
			type TestStruct struct {
				Timestamp time.Time `json:"timestamp"`
			}

			now := time.Now().UTC()
			doc := basic.Document{
				"timestamp": now,
			}

			targetType := reflect.TypeOf(TestStruct{})
			result, err := supervisor.FromDocument(doc, targetType)
			Expect(err).NotTo(HaveOccurred())

			output, ok := result.(TestStruct)
			Expect(ok).To(BeTrue())
			Expect(output.Timestamp.Unix()).To(Equal(now.Unix()))
		})
	})

	Describe("round-trip conversion", func() {
		It("preserves data through to/from Document cycle", func() {
			type TestStruct struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			}

			original := TestStruct{
				Name:  "test",
				Count: 42,
			}

			doc, err := supervisor.ToDocument(original)
			Expect(err).NotTo(HaveOccurred())

			targetType := reflect.TypeOf(TestStruct{})
			result, err := supervisor.FromDocument(doc, targetType)
			Expect(err).NotTo(HaveOccurred())

			final, ok := result.(TestStruct)
			Expect(ok).To(BeTrue())
			Expect(final).To(Equal(original))
		})

		It("preserves ContainerObservedState through round-trip", func() {
			now := time.Now().UTC().Truncate(time.Second)
			original := container.ContainerObservedState{
				CPUUsageMCores:   1500.0,
				CPUCoreCount:     4,
				CgroupCores:      2.0,
				ThrottleRatio:    0.05,
				IsThrottled:      false,
				MemoryUsedBytes:  1024 * 1024 * 512,
				MemoryTotalBytes: 1024 * 1024 * 1024 * 2,
				DiskUsedBytes:    1024 * 1024 * 1024 * 10,
				DiskTotalBytes:   1024 * 1024 * 1024 * 50,
				CollectedAt:      now,
				ObservedThresholds: container.HealthThresholds{
					CPUHighPercent:        70.0,
					CPUMediumPercent:      60.0,
					MemoryHighPercent:     80.0,
					MemoryMediumPercent:   70.0,
					DiskHighPercent:       85.0,
					DiskMediumPercent:     75.0,
					CPUThrottleRatioLimit: 0.10,
				},
			}

			doc, err := supervisor.ToDocument(original)
			Expect(err).NotTo(HaveOccurred())

			targetType := reflect.TypeOf(container.ContainerObservedState{})
			result, err := supervisor.FromDocument(doc, targetType)
			Expect(err).NotTo(HaveOccurred())

			final, ok := result.(container.ContainerObservedState)
			Expect(ok).To(BeTrue())
			Expect(final.CPUUsageMCores).To(Equal(original.CPUUsageMCores))
			Expect(final.CPUCoreCount).To(Equal(original.CPUCoreCount))
			Expect(final.CgroupCores).To(Equal(original.CgroupCores))
			Expect(final.ThrottleRatio).To(Equal(original.ThrottleRatio))
			Expect(final.IsThrottled).To(Equal(original.IsThrottled))
			Expect(final.MemoryUsedBytes).To(Equal(original.MemoryUsedBytes))
			Expect(final.MemoryTotalBytes).To(Equal(original.MemoryTotalBytes))
			Expect(final.DiskUsedBytes).To(Equal(original.DiskUsedBytes))
			Expect(final.DiskTotalBytes).To(Equal(original.DiskTotalBytes))
			Expect(final.CollectedAt.Unix()).To(Equal(original.CollectedAt.Unix()))
			Expect(final.ObservedThresholds).To(Equal(original.ObservedThresholds))
		})
	})
})

func BenchmarkToDocument(b *testing.B) {
	b.Run("SimpleStruct", func(b *testing.B) {
		type SimpleStruct struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		input := SimpleStruct{
			Name:  "test",
			Count: 42,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = supervisor.ToDocument(input)
		}
	})

	b.Run("ContainerObservedState", func(b *testing.B) {
		now := time.Now().UTC().Truncate(time.Second)
		state := container.ContainerObservedState{
			CPUUsageMCores:   1500.0,
			CPUCoreCount:     4,
			CgroupCores:      2.0,
			ThrottleRatio:    0.05,
			IsThrottled:      false,
			MemoryUsedBytes:  1024 * 1024 * 512,
			MemoryTotalBytes: 1024 * 1024 * 1024 * 2,
			DiskUsedBytes:    1024 * 1024 * 1024 * 10,
			DiskTotalBytes:   1024 * 1024 * 1024 * 50,
			CollectedAt:      now,
			ObservedThresholds: container.HealthThresholds{
				CPUHighPercent:        70.0,
				CPUMediumPercent:      60.0,
				MemoryHighPercent:     80.0,
				MemoryMediumPercent:   70.0,
				DiskHighPercent:       85.0,
				DiskMediumPercent:     75.0,
				CPUThrottleRatioLimit: 0.10,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = supervisor.ToDocument(state)
		}
	})
}

func BenchmarkFromDocument(b *testing.B) {
	b.Run("SimpleStruct", func(b *testing.B) {
		type SimpleStruct struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		doc := basic.Document{
			"name":  "test",
			"count": 42,
		}
		targetType := reflect.TypeOf(SimpleStruct{})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = supervisor.FromDocument(doc, targetType)
		}
	})

	b.Run("ContainerObservedState", func(b *testing.B) {
		now := time.Now().UTC().Truncate(time.Second)
		state := container.ContainerObservedState{
			CPUUsageMCores:   1500.0,
			CPUCoreCount:     4,
			CgroupCores:      2.0,
			ThrottleRatio:    0.05,
			IsThrottled:      false,
			MemoryUsedBytes:  1024 * 1024 * 512,
			MemoryTotalBytes: 1024 * 1024 * 1024 * 2,
			DiskUsedBytes:    1024 * 1024 * 1024 * 10,
			DiskTotalBytes:   1024 * 1024 * 1024 * 50,
			CollectedAt:      now,
			ObservedThresholds: container.HealthThresholds{
				CPUHighPercent:        70.0,
				CPUMediumPercent:      60.0,
				MemoryHighPercent:     80.0,
				MemoryMediumPercent:   70.0,
				DiskHighPercent:       85.0,
				DiskMediumPercent:     75.0,
				CPUThrottleRatioLimit: 0.10,
			},
		}

		doc, _ := supervisor.ToDocument(state)
		targetType := reflect.TypeOf(container.ContainerObservedState{})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = supervisor.FromDocument(doc, targetType)
		}
	})
}

func BenchmarkRoundTrip(b *testing.B) {
	b.Run("SimpleStruct", func(b *testing.B) {
		type SimpleStruct struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		input := SimpleStruct{
			Name:  "test",
			Count: 42,
		}
		targetType := reflect.TypeOf(SimpleStruct{})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			doc, _ := supervisor.ToDocument(input)
			_, _ = supervisor.FromDocument(doc, targetType)
		}
	})

	b.Run("ContainerObservedState", func(b *testing.B) {
		now := time.Now().UTC().Truncate(time.Second)
		state := container.ContainerObservedState{
			CPUUsageMCores:   1500.0,
			CPUCoreCount:     4,
			CgroupCores:      2.0,
			ThrottleRatio:    0.05,
			IsThrottled:      false,
			MemoryUsedBytes:  1024 * 1024 * 512,
			MemoryTotalBytes: 1024 * 1024 * 1024 * 2,
			DiskUsedBytes:    1024 * 1024 * 1024 * 10,
			DiskTotalBytes:   1024 * 1024 * 1024 * 50,
			CollectedAt:      now,
			ObservedThresholds: container.HealthThresholds{
				CPUHighPercent:        70.0,
				CPUMediumPercent:      60.0,
				MemoryHighPercent:     80.0,
				MemoryMediumPercent:   70.0,
				DiskHighPercent:       85.0,
				DiskMediumPercent:     75.0,
				CPUThrottleRatioLimit: 0.10,
			},
		}

		targetType := reflect.TypeOf(container.ContainerObservedState{})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			doc, _ := supervisor.ToDocument(state)
			_, _ = supervisor.FromDocument(doc, targetType)
		}
	})
}

func BenchmarkDirectFieldAccess(b *testing.B) {
	b.Run("ManualCopy", func(b *testing.B) {
		now := time.Now().UTC().Truncate(time.Second)
		state := container.ContainerObservedState{
			CPUUsageMCores:   1500.0,
			CPUCoreCount:     4,
			CgroupCores:      2.0,
			ThrottleRatio:    0.05,
			IsThrottled:      false,
			MemoryUsedBytes:  1024 * 1024 * 512,
			MemoryTotalBytes: 1024 * 1024 * 1024 * 2,
			DiskUsedBytes:    1024 * 1024 * 1024 * 10,
			DiskTotalBytes:   1024 * 1024 * 1024 * 50,
			CollectedAt:      now,
			ObservedThresholds: container.HealthThresholds{
				CPUHighPercent:        70.0,
				CPUMediumPercent:      60.0,
				MemoryHighPercent:     80.0,
				MemoryMediumPercent:   70.0,
				DiskHighPercent:       85.0,
				DiskMediumPercent:     75.0,
				CPUThrottleRatioLimit: 0.10,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result := container.ContainerObservedState{
				CPUUsageMCores:     state.CPUUsageMCores,
				CPUCoreCount:       state.CPUCoreCount,
				CgroupCores:        state.CgroupCores,
				ThrottleRatio:      state.ThrottleRatio,
				IsThrottled:        state.IsThrottled,
				MemoryUsedBytes:    state.MemoryUsedBytes,
				MemoryTotalBytes:   state.MemoryTotalBytes,
				DiskUsedBytes:      state.DiskUsedBytes,
				DiskTotalBytes:     state.DiskTotalBytes,
				CollectedAt:        state.CollectedAt,
				ObservedThresholds: state.ObservedThresholds,
			}
			_ = result
		}
	})
}
