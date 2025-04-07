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

package latency

import (
	"sort"
	"time"

	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func CalculateLatency(latencies *expiremap.ExpireMap[time.Time, time.Duration]) models.Latency {
	var minimumDuration time.Duration
	var maximumDuration time.Duration
	var p95 time.Duration
	var p99 time.Duration
	var avgNs int64

	var durations []time.Duration

	items := latencies.Length()
	latencies.Range(func(_ time.Time, value time.Duration) bool {
		if minimumDuration == 0 || value < minimumDuration {
			minimumDuration = value
		}
		if value > maximumDuration {
			maximumDuration = value
		}
		avgNs += value.Nanoseconds()
		durations = append(durations, value)
		return true
	})

	if items > 0 {
		avgNs /= int64(items)
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})
		p95Index := int(float64(items) * 0.95)
		p99Index := int(float64(items) * 0.99)
		if p95Index >= items {
			p95Index = items - 1
		}
		if p99Index >= items {
			p99Index = items - 1
		}
		// Ensure index is within bounds
		if p95Index < 0 {
			p95Index = 0
		}
		if p99Index < 0 {
			p99Index = 0
		}

		// Only try to access durations if it's properly initialized and has items
		if len(durations) > 0 {
			p95 = durations[p95Index]
			p99 = durations[p99Index]
		}
	}

	return models.Latency{
		MinMs: float64(minimumDuration),
		MaxMs: float64(maximumDuration),
		P95Ms: float64(p95),
		P99Ms: float64(p99),
		AvgMs: float64(avgNs),
	}
}
