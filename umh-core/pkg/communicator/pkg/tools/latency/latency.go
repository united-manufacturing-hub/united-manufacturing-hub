package latency

import (
	"sort"
	"time"

	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
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
		Min: float64(minimumDuration),
		Max: float64(maximumDuration),
		P95: float64(p95),
		P99: float64(p99),
		Avg: float64(avgNs),
	}
}
